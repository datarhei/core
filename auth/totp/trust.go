package totp

import (
	"fmt"
	"os"
	"time"

	gojson "encoding/json"

	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/math/rand"
)

const defaultTrustFilepath = "/totp_trust.json"

const (
	Remember30Days = "30d"
	Remember1Year  = "1y"
)

type trustEntry struct {
	Username  string    `json:"username"`
	ExpiresAt time.Time `json:"expires_at"`
}

type trustFileData struct {
	Devices map[string]trustEntry `json:"devices"`
}

func (s *Store) loadTrust() (trustFileData, error) {
	data := trustFileData{
		Devices: map[string]trustEntry{},
	}

	_, err := s.fs.Stat(defaultTrustFilepath)
	if err != nil {
		if os.IsNotExist(err) {
			return data, nil
		}

		return data, err
	}

	jsondata, err := s.fs.ReadFile(defaultTrustFilepath)
	if err != nil {
		return data, err
	}

	if len(jsondata) == 0 {
		return data, nil
	}

	if err := gojson.Unmarshal(jsondata, &data); err != nil {
		return data, json.FormatError(jsondata, err)
	}

	if data.Devices == nil {
		data.Devices = map[string]trustEntry{}
	}

	return data, nil
}

func (s *Store) persistTrust(data trustFileData) error {
	if data.Devices == nil {
		data.Devices = map[string]trustEntry{}
	}

	jsondata, err := gojson.MarshalIndent(&data, "", "    ")
	if err != nil {
		return err
	}

	_, _, err = s.fs.WriteFileSafe(defaultTrustFilepath, jsondata)
	return err
}

func rememberDuration(remember string) (time.Duration, error) {
	switch remember {
	case Remember30Days:
		return time.Hour * 24 * 30, nil
	case Remember1Year:
		return time.Hour * 24 * 365, nil
	default:
		return 0, fmt.Errorf("invalid remember duration")
	}
}

// TrustValid reports whether a device trust token is valid for the given user.
func (s *Store) TrustValid(token, username string) bool {
	if len(token) == 0 || len(username) == 0 {
		return false
	}

	s.lock.RLock()
	defer s.lock.RUnlock()

	if !s.data.Enrolled {
		return false
	}

	data, err := s.loadTrust()
	if err != nil {
		return false
	}

	entry, ok := data.Devices[token]
	if !ok {
		return false
	}

	if entry.Username != username {
		return false
	}

	if time.Now().After(entry.ExpiresAt) {
		return false
	}

	return true
}

// IssueTrust creates a new device trust token for the given user.
func (s *Store) IssueTrust(username, remember string) (string, error) {
	duration, err := rememberDuration(remember)
	if err != nil {
		return "", err
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	if !s.data.Enrolled {
		return "", fmt.Errorf("TOTP is not enabled")
	}

	data, err := s.loadTrust()
	if err != nil {
		return "", err
	}

	now := time.Now()

	for token, entry := range data.Devices {
		if now.After(entry.ExpiresAt) || entry.Username == username {
			delete(data.Devices, token)
		}
	}

	token := rand.String(48)
	data.Devices[token] = trustEntry{
		Username:  username,
		ExpiresAt: now.Add(duration),
	}

	if err := s.persistTrust(data); err != nil {
		return "", err
	}

	return token, nil
}

func (s *Store) clearTrustLocked() error {
	return s.persistTrust(trustFileData{
		Devices: map[string]trustEntry{},
	})
}
