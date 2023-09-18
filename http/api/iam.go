package api

import (
	"time"

	"github.com/datarhei/core/v16/iam/access"
	"github.com/datarhei/core/v16/iam/identity"
)

type IAMUser struct {
	CreatedAt int64       `json:"created_at" format:"int64"`
	UpdatedAt int64       `json:"updated_at" format:"int64"`
	Name      string      `json:"name"`
	Alias     string      `json:"alias"`
	Superuser bool        `json:"superuser"`
	Auth      IAMUserAuth `json:"auth"`
	Policies  []IAMPolicy `json:"policies"`
}

func (u *IAMUser) Marshal(user identity.User, policies []access.Policy) {
	u.CreatedAt = user.CreatedAt.Unix()
	u.UpdatedAt = user.UpdatedAt.Unix()
	u.Name = user.Name
	u.Alias = user.Alias
	u.Superuser = user.Superuser
	u.Auth = IAMUserAuth{
		API: IAMUserAuthAPI{
			Password: user.Auth.API.Password,
			Auth0: IAMUserAuthAPIAuth0{
				User: user.Auth.API.Auth0.User,
				Tenant: IAMAuth0Tenant{
					Domain:   user.Auth.API.Auth0.Tenant.Domain,
					Audience: user.Auth.API.Auth0.Tenant.Audience,
					ClientID: user.Auth.API.Auth0.Tenant.ClientID,
				},
			},
		},
		Services: IAMUserAuthServices{
			Basic:   user.Auth.Services.Basic,
			Token:   user.Auth.Services.Token,
			Session: user.Auth.Services.Session,
		},
	}

	for _, p := range policies {
		u.Policies = append(u.Policies, IAMPolicy{
			Domain:   p.Domain,
			Resource: p.Resource,
			Actions:  p.Actions,
		})
	}
}

func (u *IAMUser) Unmarshal() (identity.User, []access.Policy) {
	iamuser := identity.User{
		CreatedAt: time.Unix(u.CreatedAt, 0),
		UpdatedAt: time.Unix(u.UpdatedAt, 0),
		Name:      u.Name,
		Alias:     u.Alias,
		Superuser: u.Superuser,
		Auth: identity.UserAuth{
			API: identity.UserAuthAPI{
				Password: u.Auth.API.Password,
				Auth0: identity.UserAuthAPIAuth0{
					User: u.Auth.API.Auth0.User,
					Tenant: identity.Auth0Tenant{
						Domain:   u.Auth.API.Auth0.Tenant.Domain,
						Audience: u.Auth.API.Auth0.Tenant.Audience,
						ClientID: u.Auth.API.Auth0.Tenant.ClientID,
					},
				},
			},
			Services: identity.UserAuthServices{
				Basic:   u.Auth.Services.Basic,
				Token:   u.Auth.Services.Token,
				Session: u.Auth.Services.Session,
			},
		},
	}

	iampolicies := []access.Policy{}

	for _, p := range u.Policies {
		iampolicies = append(iampolicies, access.Policy{
			Name:     u.Name,
			Domain:   p.Domain,
			Resource: p.Resource,
			Actions:  p.Actions,
		})
	}

	return iamuser, iampolicies
}

type IAMUserAuth struct {
	API      IAMUserAuthAPI      `json:"api"`
	Services IAMUserAuthServices `json:"services"`
}

type IAMUserAuthAPI struct {
	Password string              `json:"userpass"`
	Auth0    IAMUserAuthAPIAuth0 `json:"auth0"`
}

type IAMUserAuthAPIAuth0 struct {
	User   string         `json:"user"`
	Tenant IAMAuth0Tenant `json:"tenant"`
}

type IAMUserAuthServices struct {
	Basic   []string `json:"basic"`
	Token   []string `json:"token"`
	Session []string `json:"session"`
}

type IAMAuth0Tenant struct {
	Domain   string `json:"domain"`
	Audience string `json:"audience"`
	ClientID string `json:"client_id"`
}

type IAMPolicy struct {
	Name     string   `json:"name,omitempty"`
	Domain   string   `json:"domain"`
	Resource string   `json:"resource"`
	Actions  []string `json:"actions"`
}
