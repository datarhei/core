package cluster

import (
	"fmt"
	"time"

	clusteriam "github.com/datarhei/core/v16/cluster/iam"
	clusteriamadapter "github.com/datarhei/core/v16/cluster/iam/adapter"
	"github.com/datarhei/core/v16/cluster/store"
	"github.com/datarhei/core/v16/iam"
	iamaccess "github.com/datarhei/core/v16/iam/access"
	iamidentity "github.com/datarhei/core/v16/iam/identity"
)

func (c *cluster) IAM(superuser iamidentity.User, jwtRealm, jwtSecret string) (iam.IAM, error) {
	policyAdapter, err := clusteriamadapter.NewPolicyAdapter(c.store)
	if err != nil {
		return nil, fmt.Errorf("cluster policy adapter: %w", err)
	}

	identityAdapter, err := clusteriamadapter.NewIdentityAdapter(c.store)
	if err != nil {
		return nil, fmt.Errorf("cluster identitry adapter: %w", err)
	}

	iam, err := clusteriam.New(iam.Config{
		PolicyAdapter:   policyAdapter,
		IdentityAdapter: identityAdapter,
		Superuser:       superuser,
		JWTRealm:        jwtRealm,
		JWTSecret:       jwtSecret,
		Logger:          c.logger.WithComponent("IAM"),
	}, c.store)
	if err != nil {
		return nil, fmt.Errorf("cluster iam: %w", err)
	}

	return iam, nil
}

func (c *cluster) ListIdentities() (time.Time, []iamidentity.User) {
	users := c.store.IAMIdentityList()

	return users.UpdatedAt, users.Users
}

func (c *cluster) ListIdentity(name string) (time.Time, iamidentity.User, error) {
	user := c.store.IAMIdentityGet(name)

	if len(user.Users) == 0 {
		return time.Time{}, iamidentity.User{}, fmt.Errorf("not found")
	}

	return user.UpdatedAt, user.Users[0], nil
}

func (c *cluster) ListPolicies() (time.Time, []iamaccess.Policy) {
	policies := c.store.IAMPolicyList()

	return policies.UpdatedAt, policies.Policies
}

func (c *cluster) ListUserPolicies(name string) (time.Time, []iamaccess.Policy) {
	policies := c.store.IAMIdentityPolicyList(name)

	return policies.UpdatedAt, policies.Policies
}

func (c *cluster) IAMIdentityAdd(origin string, identity iamidentity.User) error {
	if err := identity.Validate(); err != nil {
		return fmt.Errorf("invalid identity: %w", err)
	}

	if !c.IsRaftLeader() {
		return c.forwarder.IAMIdentityAdd(origin, identity)
	}

	cmd := &store.Command{
		Operation: store.OpAddIdentity,
		Data: &store.CommandAddIdentity{
			Identity: identity,
		},
	}

	return c.applyCommand(cmd)
}

func (c *cluster) IAMIdentityUpdate(origin, name string, identity iamidentity.User) error {
	if !c.IsRaftLeader() {
		return c.forwarder.IAMIdentityUpdate(origin, name, identity)
	}

	cmd := &store.Command{
		Operation: store.OpUpdateIdentity,
		Data: &store.CommandUpdateIdentity{
			Name:     name,
			Identity: identity,
		},
	}

	return c.applyCommand(cmd)
}

func (c *cluster) IAMPoliciesSet(origin, name string, policies []iamaccess.Policy) error {
	if !c.IsRaftLeader() {
		return c.forwarder.IAMPoliciesSet(origin, name, policies)
	}

	cmd := &store.Command{
		Operation: store.OpSetPolicies,
		Data: &store.CommandSetPolicies{
			Name:     name,
			Policies: policies,
		},
	}

	return c.applyCommand(cmd)
}

func (c *cluster) IAMIdentityRemove(origin string, name string) error {
	if !c.IsRaftLeader() {
		return c.forwarder.IAMIdentityRemove(origin, name)
	}

	cmd := &store.Command{
		Operation: store.OpRemoveIdentity,
		Data: &store.CommandRemoveIdentity{
			Name: name,
		},
	}

	return c.applyCommand(cmd)
}
