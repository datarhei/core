package identity

import (
	"sync"

	"github.com/datarhei/core/v16/iam/jwks"
)

type Auth0Tenant struct {
	Domain   string `json:"domain"`
	Audience string `json:"audience"`
	ClientID string `json:"client_id"`
}

func (t *Auth0Tenant) key() string {
	return t.Domain + t.Audience
}

type auth0Tenant struct {
	domain    string
	issuer    string
	audience  string
	clientIDs []string
	certs     jwks.JWKS

	lock sync.Mutex
}

func newAuth0Tenant(tenant Auth0Tenant) (*auth0Tenant, error) {
	t := &auth0Tenant{
		domain:    tenant.Domain,
		issuer:    "https://" + tenant.Domain + "/",
		audience:  tenant.Audience,
		clientIDs: []string{tenant.ClientID},
		certs:     nil,
	}

	url := t.issuer + ".well-known/jwks.json"
	certs, err := jwks.NewFromURL(url, jwks.Config{})
	if err != nil {
		return nil, err
	}

	t.certs = certs

	return t, nil
}

func (a *auth0Tenant) Cancel() {
	a.certs.Cancel()
}

func (a *auth0Tenant) AddClientID(clientid string) {
	a.lock.Lock()
	defer a.lock.Unlock()

	found := false
	for _, id := range a.clientIDs {
		if id == clientid {
			found = true
			break
		}
	}

	if found {
		return
	}

	a.clientIDs = append(a.clientIDs, clientid)
}

func (a *auth0Tenant) RemoveClientID(clientid string) {
	a.lock.Lock()
	defer a.lock.Unlock()

	clientids := []string{}

	for _, id := range a.clientIDs {
		if id == clientid {
			continue
		}

		clientids = append(clientids, id)
	}

	a.clientIDs = clientids
}
