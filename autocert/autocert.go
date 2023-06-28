package autocert

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/datarhei/core/v16/log"

	"github.com/caddyserver/certmagic"
	"go.uber.org/zap"
)

type Manager interface {
	AcquireCertificates(ctx context.Context, listenAddress string, hostnames []string) error
	GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error)
	HTTPChallengeHandler(h http.Handler) http.Handler
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

	logger log.Logger
}

func New(config Config) (Manager, error) {
	m := &manager{
		logger: config.Logger,
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

func (m *manager) HTTPChallengeHandler(h http.Handler) http.Handler {
	acme := m.config.Issuers[0].(*certmagic.ACMEIssuer)
	return acme.HTTPChallengeHandler(h)
}

func (m *manager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	return m.config.GetCertificate(hello)
}

func (m *manager) AcquireCertificates(ctx context.Context, listenAddress string, hostnames []string) error {
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

	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		tempserver.ListenAndServe()
		wg.Done()
	}()

	var certerr error

	// Get the certificates
	logger := m.logger.WithField("hostnames", hostnames)
	logger.Info().Log("Acquiring certificate ...")

	err := m.config.ManageSync(ctx, hostnames)

	if err != nil {
		certerr = fmt.Errorf("failed to acquire certificate for %s: %w", strings.Join(hostnames, ","), err)
	} else {
		logger.Info().Log("Successfully acquired certificate")
	}

	// Shut down the temporary http server
	tempserver.Close()

	wg.Wait()

	return certerr
}

func ProxyHTTPChallenge(ctx context.Context, listenAddress string, target *url.URL) error {
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Start temporary http server on configured port
	tempserver := &http.Server{
		Addr:           listenAddress,
		Handler:        proxy,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		tempserver.ListenAndServe()
		wg.Done()
	}()

	<-ctx.Done()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	tempserver.Shutdown(ctx)
	cancel()

	return nil
}
