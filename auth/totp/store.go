package totp

import (
	"fmt"
	"os"
	"sync"

	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/io/fs"

	gojson "encoding/json"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

const defaultFilepath = "/totp.json"

// Config holds the configuration for a TOTP store.
type Config struct {
	Filesystem fs.Filesystem
	Filepath   string
	Issuer     string
}

// Status describes the current TOTP enrollment state.
type Status struct {
	Enrolled bool `json:"enrolled"`
	Pending  bool `json:"pending"`
}

// Setup holds the data returned when starting TOTP enrollment.
type Setup struct {
	Secret string `json:"secret"`
	URI    string `json:"uri"`
}

type fileData struct {
	Enrolled bool   `json:"enrolled"`
	Pending  bool   `json:"pending"`
	Secret   string `json:"secret,omitempty"`
	Username string `json:"username,omitempty"`
}

// Store persists TOTP enrollment state and validates codes.
type Store struct {
	fs     fs.Filesystem
	path   string
	issuer string

	lock sync.RWMutex
	data fileData
}

// NewStore creates a TOTP store backed by the given filesystem.
func NewStore(config Config) (*Store, error) {
	if config.Filesystem == nil {
		return nil, fmt.Errorf("no valid filesystem provided")
	}

	path := config.Filepath
	if len(path) == 0 {
		path = defaultFilepath
	}

	issuer := config.Issuer
	if len(issuer) == 0 {
		issuer = "Restreamer"
	}

	s := &Store{
		fs:     config.Filesystem,
		path:   path,
		issuer: issuer,
	}

	if err := s.load(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *Store) load() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	_, err := s.fs.Stat(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.data = fileData{}
			return nil
		}

		return err
	}

	jsondata, err := s.fs.ReadFile(s.path)
	if err != nil {
		return err
	}

	if len(jsondata) == 0 {
		s.data = fileData{}
		return nil
	}

	if err := gojson.Unmarshal(jsondata, &s.data); err != nil {
		return json.FormatError(jsondata, err)
	}

	return nil
}

func (s *Store) persistLocked() error {
	jsondata, err := gojson.MarshalIndent(&s.data, "", "    ")
	if err != nil {
		return err
	}

	_, _, err = s.fs.WriteFileSafe(s.path, jsondata)
	return err
}

// Status returns the current enrollment state.
func (s *Store) Status() Status {
	s.lock.RLock()
	defer s.lock.RUnlock()

	return Status{
		Enrolled: s.data.Enrolled,
		Pending:  s.data.Pending,
	}
}

// Enrolled reports whether TOTP is active for login.
func (s *Store) Enrolled() bool {
	s.lock.RLock()
	defer s.lock.RUnlock()

	return s.data.Enrolled
}

// Setup starts TOTP enrollment and returns the secret and otpauth URI.
func (s *Store) Setup(username string) (Setup, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.data.Enrolled {
		return Setup{}, fmt.Errorf("TOTP is already enabled")
	}

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      s.issuer,
		AccountName: username,
		Period:      30,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return Setup{}, fmt.Errorf("failed to generate TOTP secret: %w", err)
	}

	s.data = fileData{
		Pending:  true,
		Secret:   key.Secret(),
		Username: username,
	}

	if err := s.persistLocked(); err != nil {
		return Setup{}, err
	}

	return Setup{
		Secret: key.Secret(),
		URI:    key.URL(),
	}, nil
}

// Enable confirms enrollment with a valid code from the authenticator app.
func (s *Store) Enable(code string) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.data.Enrolled {
		return fmt.Errorf("TOTP is already enabled")
	}

	if !s.data.Pending || len(s.data.Secret) == 0 {
		return fmt.Errorf("TOTP setup has not been started")
	}

	if !totp.Validate(code, s.data.Secret) {
		return fmt.Errorf("invalid TOTP code")
	}

	s.data.Enrolled = true
	s.data.Pending = false

	return s.persistLocked()
}

// Disable removes TOTP enrollment after validating the current code.
func (s *Store) Disable(code string) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if !s.data.Enrolled {
		return fmt.Errorf("TOTP is not enabled")
	}

	if !totp.Validate(code, s.data.Secret) {
		return fmt.Errorf("invalid TOTP code")
	}

	s.data = fileData{}

	if err := s.persistLocked(); err != nil {
		return err
	}

	return s.clearTrustLocked()
}

// Validate checks a TOTP code against the enrolled secret.
func (s *Store) Validate(code string) bool {
	s.lock.RLock()
	defer s.lock.RUnlock()

	if !s.data.Enrolled || len(s.data.Secret) == 0 {
		return false
	}

	return totp.Validate(code, s.data.Secret)
}
