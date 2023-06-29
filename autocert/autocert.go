package autocert

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/datarhei/core/v16/log"

	"github.com/caddyserver/certmagic"
	"github.com/klauspost/cpuid/v2"
	"go.uber.org/zap"
)

type Manager interface {
	AcquireCertificates(ctx context.Context, hostnames []string) error
	ManageCertificates(ctx context.Context, hostnames []string) error
	HTTPChallengeResolver(ctx context.Context, listenAddress string) error
	HTTPChallengeHandler(h http.Handler) http.Handler
	GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error)
	TLSConfig() *tls.Config
	ManagedNames() []string
}

type Config struct {
	Storage         certmagic.Storage
	DefaultHostname string
	EmailAddress    string
	IsProduction    bool
	Logger          log.Logger
}

type manager struct {
	config *certmagic.Config

	hostnames []string
	lock      sync.Mutex

	logger log.Logger
}

func New(config Config) (Manager, error) {
	m := &manager{
		hostnames: []string{},
		logger:    config.Logger,
	}

	if m.logger == nil {
		m.logger = log.New("")
	}

	certmagic.Default.Storage = config.Storage
	certmagic.Default.DefaultServerName = config.DefaultHostname
	certmagic.Default.Logger = zap.NewNop()

	ca := certmagic.LetsEncryptStagingCA
	if config.IsProduction {
		ca = certmagic.LetsEncryptProductionCA
		m.logger.Info().WithField("ca", ca).Log("Using production CA")
	} else {
		m.logger.Info().WithField("ca", ca).Log("Using staging CA")
	}

	certmagic.DefaultACME.Agreed = true
	certmagic.DefaultACME.Email = config.EmailAddress
	certmagic.DefaultACME.CA = ca
	certmagic.DefaultACME.DisableHTTPChallenge = false
	certmagic.DefaultACME.DisableTLSALPNChallenge = true
	certmagic.DefaultACME.Logger = zap.NewNop()

	magic := certmagic.NewDefault()
	acme := certmagic.NewACMEIssuer(magic, certmagic.DefaultACME)
	acme.Logger = zap.NewNop()

	magic.Issuers = []certmagic.Issuer{acme}
	magic.Logger = zap.NewNop()

	m.config = magic

	return m, nil
}

// HTTPChallengeHandler wraps h in a handler that can solve the ACME
// HTTP challenge. cfg is required, and it must have a certificate
// cache backed by a functional storage facility, since that is where
// the challenge state is stored between initiation and solution.
//
// If a request is not an ACME HTTP challenge, h will be invoked.
func (m *manager) HTTPChallengeHandler(h http.Handler) http.Handler {
	acme := m.config.Issuers[0].(*certmagic.ACMEIssuer)
	return acme.HTTPChallengeHandler(h)
}

// GetCertificate gets a certificate to satisfy clientHello. In getting
// the certificate, it abides the rules and settings defined in the Config
// that matches clientHello.ServerName. It tries to get certificates in
// this order:
//
// 1. Exact match in the in-memory cache
// 2. Wildcard match in the in-memory cache
// 3. Managers (if any)
// 4. Storage (if on-demand is enabled)
// 5. Issuers (if on-demand is enabled)
//
// This method is safe for use as a tls.Config.GetCertificate callback.
//
// GetCertificate will run in a new context.
func (m *manager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	return m.config.GetCertificate(hello)
}

// HTTPChallengeResolver starts a http server that responds to HTTP challenge requests and returns
// as soon as the server is running. Use the context to stop the server.
func (m *manager) HTTPChallengeResolver(ctx context.Context, listenAddress string) error {
	acme := m.config.Issuers[0].(*certmagic.ACMEIssuer)

	// Start temporary http server on configured port
	tempserver := &http.Server{
		Addr: listenAddress,
		Handler: acme.HTTPChallengeHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})),
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	errorCh := make(chan error, 1)

	wg := sync.WaitGroup{}
	wg.Add(1)

	go func(errorCh chan<- error) {
		wg.Done()
		errorCh <- tempserver.ListenAndServe()
	}(errorCh)

	wg.Wait()

	// Wait for an error
	select {
	case err := <-errorCh:
		return err
	case <-time.After(3 * time.Second):
		break
	}

	go func(ctx context.Context) {
		<-ctx.Done()
		tempserver.Close()

		// Drain and close the channel
		select {
		case <-errorCh:
		default:
		}

		close(errorCh)
	}(ctx)

	return nil
}

// AcquireCertificates tries to acquire the certificates for the given hostnames synchronously.
func (m *manager) AcquireCertificates(ctx context.Context, hostnames []string) error {
	m.lock.Lock()
	added, removed := diffStringSlice(hostnames, m.hostnames)
	m.lock.Unlock()

	var err error

	if len(added) != 0 {
		// Get the certificates
		m.logger.WithField("hostnames", added).Info().Log("Acquiring certificates ...")

		err = m.config.ManageSync(ctx, added)
		if err != nil {
			return fmt.Errorf("failed to acquire certificate for %s: %w", strings.Join(added, ","), err)
		}

		m.logger.WithField("hostnames", added).Info().Log("Successfully acquired certificate")
	}

	if len(removed) != 0 {
		m.logger.WithField("hostnames", removed).Info().Log("Unmanage certificates")
		m.config.Unmanage(removed)
	}

	m.lock.Lock()
	m.hostnames = make([]string, len(hostnames))
	copy(m.hostnames, hostnames)
	m.lock.Unlock()

	return err
}

// ManageCertificates is the same as AcquireCertificates but it does it in the background.
func (m *manager) ManageCertificates(ctx context.Context, hostnames []string) error {
	m.lock.Lock()
	added, removed := diffStringSlice(hostnames, m.hostnames)
	m.hostnames = make([]string, len(hostnames))
	copy(m.hostnames, hostnames)
	m.lock.Unlock()

	if len(removed) != 0 {
		m.logger.WithField("hostnames", removed).Info().Log("Unmanage certificates")
		m.config.Unmanage(removed)
	}

	if len(added) == 0 {
		return nil
	}

	m.logger.WithField("hostnames", added).Info().Log("Acquiring certificates")

	return m.config.ManageAsync(ctx, added)
}

// ManagedNames returns a list of the currently managed domain names.
func (m *manager) ManagedNames() []string {
	m.lock.Lock()
	defer m.lock.Unlock()

	hostnames := make([]string, len(m.hostnames))
	copy(hostnames, m.hostnames)

	return hostnames
}

// TLSConfig is an opinionated method that returns a recommended, modern
// TLS configuration that can be used to configure TLS listeners. Aside
// from safe, modern defaults, this method sets one critical field on the
// TLS config which is required to enable automatic certificate
// management: GetCertificate.
//
// The GetCertificate field is necessary to get certificates from memory
// or storage, including both manual and automated certificates. You
// should only change this field if you know what you are doing.
func (m *manager) TLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: m.GetCertificate,

		// the rest recommended for modern TLS servers
		MinVersion: tls.VersionTLS12,
		CurvePreferences: []tls.CurveID{
			tls.X25519,
			tls.CurveP256,
		},
		CipherSuites:             preferredDefaultCipherSuites(),
		PreferServerCipherSuites: true,
	}
}

// preferredDefaultCipherSuites returns an appropriate
// cipher suite to use depending on hardware support
// for AES-NI.
//
// See https://github.com/mholt/caddy/issues/1674
// Copied from https://github.com/caddyserver/certmagic/blob/d8e706f9b5011ecbaf20d3c1641e5446ad453613/crypto.go#L299
func preferredDefaultCipherSuites() []uint16 {
	if cpuid.CPU.Supports(cpuid.AESNI) {
		return defaultCiphersPreferAES
	}
	return defaultCiphersPreferChaCha
}

var (
	defaultCiphersPreferAES = []uint16{
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
	}
	defaultCiphersPreferChaCha = []uint16{
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	}
)

// diffHostnames returns a list of newly added hostnames and a list of removed hostnames based
// the provided list and the list of currently managed hostnames.
func diffStringSlice(next, current []string) ([]string, []string) {
	added, removed := []string{}, []string{}

	currentMap := map[string]struct{}{}

	for _, name := range current {
		currentMap[name] = struct{}{}
	}

	for _, name := range next {
		if _, ok := currentMap[name]; ok {
			delete(currentMap, name)
			continue
		}

		added = append(added, name)
	}

	for name := range currentMap {
		removed = append(removed, name)
	}

	return added, removed
}
