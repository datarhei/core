package iam

import (
	"errors"

	"github.com/datarhei/core/v16/cluster/store"
	"github.com/datarhei/core/v16/iam"
	"github.com/datarhei/core/v16/iam/access"
	"github.com/datarhei/core/v16/iam/identity"
	"github.com/datarhei/core/v16/log"
)

var ErrClusterMode = errors.New("not available in cluster mode")

type manager struct {
	iam    iam.IAM
	store  store.Store
	logger log.Logger
}

func New(config iam.Config, store store.Store) (iam.IAM, error) {
	mngr, err := iam.New(config)
	if err != nil {
		return nil, err
	}

	m := &manager{
		iam:    mngr,
		store:  store,
		logger: config.Logger,
	}

	if m.logger == nil {
		m.logger = log.New("")
	}

	store.OnApply(m.apply)

	return m, nil
}

func (m *manager) apply(op store.Operation) {
	m.logger.Debug().WithField("operation", string(op)).Log("Applying action on operation")

	var err error

	switch op {
	case store.OpAddIdentity:
		err = m.ReloadIndentities()
	case store.OpUpdateIdentity:
		err = m.ReloadIndentities()
	case store.OpRemoveIdentity:
		err = m.ReloadIndentities()
	case store.OpSetPolicies:
		err = m.ReloadPolicies()
	}

	if err != nil {
		m.logger.Error().WithError(err).WithField("operation", string(op)).Log("")
	}
}

func (m *manager) Enforce(name, domain, resource, action string) bool {
	return m.iam.Enforce(name, domain, resource, action)
}

func (m *manager) HasDomain(domain string) bool {
	return m.iam.HasDomain(domain)
}

func (m *manager) ListDomains() []string {
	return m.iam.ListDomains()
}

func (m *manager) HasPolicy(name, domain, resource string, actions []string) bool {
	return m.iam.HasPolicy(name, domain, resource, actions)
}

func (m *manager) AddPolicy(name, domain, resource string, actions []string) error {
	return ErrClusterMode
}

func (m *manager) RemovePolicy(name, domain, resource string, actions []string) error {
	return ErrClusterMode
}

func (m *manager) ListPolicies(name, domain, resource string, actions []string) []access.Policy {
	return m.iam.ListPolicies(name, domain, resource, actions)
}

func (m *manager) ReloadPolicies() error {
	m.logger.Info().Log("Reloading policies")
	return m.iam.ReloadPolicies()
}

func (m *manager) Validators() []string {
	return m.iam.Validators()
}

func (m *manager) CreateIdentity(u identity.User) error {
	return ErrClusterMode
}

func (m *manager) GetIdentity(name string) (identity.User, error) {
	return m.iam.GetIdentity(name)
}

func (m *manager) UpdateIdentity(name string, u identity.User) error {
	return ErrClusterMode
}

func (m *manager) DeleteIdentity(name string) error {
	return ErrClusterMode
}

func (m *manager) ListIdentities() []identity.User {
	return m.iam.ListIdentities()
}

func (m *manager) ReloadIndentities() error {
	m.logger.Info().Log("Reloading identities")
	return m.iam.ReloadIndentities()
}

func (m *manager) GetVerifier(name string) (identity.Verifier, error) {
	return m.iam.GetVerifier(name)
}
func (m *manager) GetVerifierFromAuth0(name string) (identity.Verifier, error) {
	return m.iam.GetVerifierFromAuth0(name)
}

func (m *manager) GetDefaultVerifier() identity.Verifier {
	return m.iam.GetDefaultVerifier()
}

func (m *manager) CreateJWT(name string) (string, string, error) {
	return m.iam.CreateJWT(name)
}

func (m *manager) Close() {}
