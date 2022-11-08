package value

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// array of auth0 tenants

type Auth0Tenant struct {
	Domain   string   `json:"domain"`
	Audience string   `json:"audience"`
	ClientID string   `json:"clientid"`
	Users    []string `json:"users"`
}

type TenantList struct {
	p         *[]Auth0Tenant
	separator string
}

func NewTenantList(p *[]Auth0Tenant, val []Auth0Tenant, separator string) *TenantList {
	v := &TenantList{
		p:         p,
		separator: separator,
	}

	*p = val

	return v
}

func (s *TenantList) Set(val string) error {
	list := []Auth0Tenant{}

	for i, elm := range strings.Split(val, s.separator) {
		data, err := base64.StdEncoding.DecodeString(elm)
		if err != nil {
			return fmt.Errorf("invalid base64 encoding of tenant %d: %w", i, err)
		}

		t := Auth0Tenant{}
		if err := json.Unmarshal(data, &t); err != nil {
			return fmt.Errorf("invalid JSON in tenant %d: %w", i, err)
		}

		list = append(list, t)
	}

	*s.p = list

	return nil
}

func (s *TenantList) String() string {
	if s.IsEmpty() {
		return "(empty)"
	}

	list := []string{}

	for _, t := range *s.p {
		list = append(list, fmt.Sprintf("%s (%d users)", t.Domain, len(t.Users)))
	}

	return strings.Join(list, ",")
}

func (s *TenantList) Validate() error {
	for i, t := range *s.p {
		if len(t.Domain) == 0 {
			return fmt.Errorf("the domain for tenant %d is missing", i)
		}

		if len(t.Audience) == 0 {
			return fmt.Errorf("the audience for tenant %d is missing", i)
		}
	}

	return nil
}

func (s *TenantList) IsEmpty() bool {
	return len(*s.p) == 0
}
