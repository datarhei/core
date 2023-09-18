package identity

import (
	"fmt"
	"strings"
	"time"

	"github.com/datarhei/core/v16/slices"
)

type User struct {
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Name      string    `json:"name"`
	Alias     string    `json:"alias"`
	Superuser bool      `json:"superuser"`
	Auth      UserAuth  `json:"auth"`
}

type UserAuth struct {
	API      UserAuthAPI      `json:"api"`
	Services UserAuthServices `json:"services"`
}

type UserAuthAPI struct {
	Password string           `json:"password"`
	Auth0    UserAuthAPIAuth0 `json:"auth0"`
}

type UserAuthAPIAuth0 struct {
	User   string      `json:"user"`
	Tenant Auth0Tenant `json:"tenant"`
}

type UserAuthServices struct {
	Basic   []string `json:"basic"`   // Passwords for BasicAuth
	Token   []string `json:"token"`   // Tokens/Streamkey for RTMP and SRT
	Session []string `json:"session"` // Secrets for session JWT
}

func (u *User) Validate() error {
	if len(u.Name) == 0 {
		return fmt.Errorf("a name is required")
	}

	if strings.HasPrefix(u.Name, "$") {
		return fmt.Errorf("name is not allowed to start with $")
	}

	if len(u.Alias) != 0 {
		if strings.HasPrefix(u.Alias, "$") {
			return fmt.Errorf("alias is not allowed to start with $")
		}
	}

	if len(u.Auth.API.Auth0.User) != 0 {
		t, err := newAuth0Tenant(u.Auth.API.Auth0.Tenant)
		if err != nil {
			return fmt.Errorf("auth0: %w", err)
		}

		t.Cancel()
	}

	return nil
}

func (u *User) marshalIdentity() *identity {
	i := &identity{
		user: u.clone(),
	}

	return i
}

func (u *User) clone() User {
	user := *u

	user.Auth.Services.Basic = slices.Copy(u.Auth.Services.Basic)
	user.Auth.Services.Token = slices.Copy(u.Auth.Services.Token)
	user.Auth.Services.Session = slices.Copy(u.Auth.Services.Session)

	return user
}

type UserList interface {
	Add(u User) error
	Get(nameOrAlias string) (User, error)
	Update(nameOrAlias string, u User) error
	Delete(nameorAlias string)
	List() []User
}

type userlist struct {
	namesUserMap map[string]string
	auth0UserMap map[string]string
	user         map[string]User
}

func NewUserList() UserList {
	return &userlist{
		namesUserMap: map[string]string{},
		auth0UserMap: map[string]string{},
		user:         map[string]User{},
	}
}

// Add implements UserList.
func (ul *userlist) Add(u User) error {
	if err := u.Validate(); err != nil {
		return fmt.Errorf("invalid user: %w", err)
	}

	if _, ok := ul.namesUserMap[u.Name]; ok {
		return fmt.Errorf("the name '%s' is already in use", u.Name)
	}

	if len(u.Alias) != 0 {
		if _, ok := ul.namesUserMap[u.Alias]; ok {
			return fmt.Errorf("the alias '%s' is already in use", u.Alias)
		}
	}

	if len(u.Auth.API.Auth0.User) != 0 {
		if name, ok := ul.auth0UserMap[u.Auth.API.Auth0.User]; ok {
			return fmt.Errorf("the Auth0 user has already an identity (%s)", name)
		}
	}

	u = u.clone()

	ul.namesUserMap[u.Name] = u.Name
	if len(u.Alias) != 0 {
		ul.namesUserMap[u.Alias] = u.Name
	}
	if len(u.Auth.API.Auth0.User) != 0 {
		ul.auth0UserMap[u.Auth.API.Auth0.User] = u.Name
	}

	ul.user[u.Name] = u

	return nil
}

func (ul *userlist) Get(nameOrAlias string) (User, error) {
	name, ok := ul.namesUserMap[nameOrAlias]
	if !ok {
		return User{}, fmt.Errorf("user not found")
	}

	u, ok := ul.user[name]
	if !ok {
		return User{}, fmt.Errorf("user not found")
	}

	return u.clone(), nil
}

// Delete implements UserList.
func (ul *userlist) Delete(nameOrAlias string) {
	name, ok := ul.namesUserMap[nameOrAlias]
	if !ok {
		return
	}

	u, ok := ul.user[name]
	if !ok {
		delete(ul.namesUserMap, nameOrAlias)
		return
	}

	delete(ul.namesUserMap, u.Name)
	delete(ul.namesUserMap, u.Alias)
	delete(ul.auth0UserMap, u.Auth.API.Auth0.User)
	delete(ul.user, u.Name)
}

// List implements UserList.
func (ul *userlist) List() []User {
	user := []User{}

	for _, u := range ul.user {
		user = append(user, u.clone())
	}

	return user
}

// Update implements UserList.
func (ul *userlist) Update(nameOrAlias string, u User) error {
	if err := u.Validate(); err != nil {
		return fmt.Errorf("invalid user: %w", err)
	}

	name, ok := ul.namesUserMap[nameOrAlias]
	if !ok {
		return fmt.Errorf("user with the name or alias '%s' not found", nameOrAlias)
	}

	oldUser, ok := ul.user[name]
	if !ok {
		return fmt.Errorf("user with the name '%s' not found", name)
	}

	delete(ul.namesUserMap, oldUser.Name)
	delete(ul.namesUserMap, oldUser.Alias)
	delete(ul.auth0UserMap, oldUser.Auth.API.Auth0.User)

	if _, ok := ul.namesUserMap[u.Name]; ok {
		ul.namesUserMap[oldUser.Name] = oldUser.Name
		if len(oldUser.Alias) != 0 {
			ul.namesUserMap[oldUser.Alias] = oldUser.Name
		}
		if len(oldUser.Auth.API.Auth0.User) != 0 {
			ul.auth0UserMap[oldUser.Auth.API.Auth0.User] = oldUser.Name
		}
		return fmt.Errorf("the name '%s' is already in use", u.Name)
	}

	if len(u.Alias) != 0 {
		if _, ok := ul.namesUserMap[u.Alias]; ok {
			ul.namesUserMap[oldUser.Name] = oldUser.Name
			if len(oldUser.Alias) != 0 {
				ul.namesUserMap[oldUser.Alias] = oldUser.Name
			}
			if len(oldUser.Auth.API.Auth0.User) != 0 {
				ul.auth0UserMap[oldUser.Auth.API.Auth0.User] = oldUser.Name
			}
			return fmt.Errorf("the alias '%s' is already in use", u.Alias)
		}
	}

	if len(u.Auth.API.Auth0.User) != 0 {
		if _, ok := ul.auth0UserMap[u.Auth.API.Auth0.User]; ok {
			ul.namesUserMap[oldUser.Name] = oldUser.Name
			if len(oldUser.Alias) != 0 {
				ul.namesUserMap[oldUser.Alias] = oldUser.Name
			}
			if len(oldUser.Auth.API.Auth0.User) != 0 {
				ul.auth0UserMap[oldUser.Auth.API.Auth0.User] = oldUser.Name
			}
			return fmt.Errorf("the Auth0 user has already an identity (%s)", u.Auth.API.Auth0.User)
		}
	}

	delete(ul.user, oldUser.Name)

	u = u.clone()

	ul.namesUserMap[u.Name] = u.Name
	if len(u.Alias) != 0 {
		ul.namesUserMap[u.Alias] = u.Name
	}
	if len(u.Auth.API.Auth0.User) != 0 {
		ul.auth0UserMap[u.Auth.API.Auth0.User] = u.Name
	}

	ul.user[u.Name] = u

	return nil
}
