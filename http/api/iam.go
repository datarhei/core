package api

import "github.com/datarhei/core/v16/iam"

type IAMUser struct {
	Name      string      `json:"name"`
	Superuser bool        `json:"superuser"`
	Auth      IAMUserAuth `json:"auth"`
	Policies  []IAMPolicy `json:"policies"`
}

func (u *IAMUser) Marshal(user iam.User, policies []iam.Policy) {
	u.Name = user.Name
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
			Basic: user.Auth.Services.Basic,
			Token: user.Auth.Services.Token,
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

func (u *IAMUser) Unmarshal() (iam.User, []iam.Policy) {
	iamuser := iam.User{
		Name:      u.Name,
		Superuser: u.Superuser,
		Auth: iam.UserAuth{
			API: iam.UserAuthAPI{
				Password: u.Auth.API.Password,
				Auth0: iam.UserAuthAPIAuth0{
					User: u.Auth.API.Auth0.User,
					Tenant: iam.Auth0Tenant{
						Domain:   u.Auth.API.Auth0.Tenant.Domain,
						Audience: u.Auth.API.Auth0.Tenant.Audience,
						ClientID: u.Auth.API.Auth0.Tenant.ClientID,
					},
				},
			},
			Services: iam.UserAuthServices{
				Basic: u.Auth.Services.Basic,
				Token: u.Auth.Services.Token,
			},
		},
	}

	iampolicies := []iam.Policy{}

	for _, p := range u.Policies {
		iampolicies = append(iampolicies, iam.Policy{
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
	Basic []string `json:"basic"`
	Token []string `json:"token"`
}

type IAMAuth0Tenant struct {
	Domain   string `json:"domain"`
	Audience string `json:"audience"`
	ClientID string `json:"client_id"`
}

type IAMPolicy struct {
	Domain   string   `json:"domain"`
	Resource string   `json:"resource"`
	Actions  []string `json:"actions"`
}
