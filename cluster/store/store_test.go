package store

import (
	"encoding/json"
	"io/fs"
	"testing"
	"time"

	"github.com/datarhei/core/v16/iam/access"
	"github.com/datarhei/core/v16/iam/identity"
	"github.com/datarhei/core/v16/restream/app"
	"github.com/hashicorp/raft"

	"github.com/stretchr/testify/require"
)

func createStore() (*store, error) {
	si, err := NewStore(Config{
		Logger: nil,
	})
	if err != nil {
		return nil, err
	}

	s := si.(*store)

	return s, nil
}

func TestCreateStore(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	require.NotNil(t, s.data.Process)
	require.NotNil(t, s.data.ProcessNodeMap)
	require.NotNil(t, s.data.Users.Users)
	require.NotNil(t, s.data.Policies.Policies)
	require.NotNil(t, s.data.Locks)
	require.NotNil(t, s.data.KVS)
}

func TestAddProcessCommand(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	config := &app.Config{
		ID:          "foobar",
		LimitCPU:    1,
		LimitMemory: 1,
	}

	err = s.applyCommand(Command{
		Operation: OpAddProcess,
		Data: CommandAddProcess{
			Config: config,
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, s.data.Process)

	p, ok := s.data.Process["foobar@"]
	require.True(t, ok)

	require.NotZero(t, p.CreatedAt)
	require.NotZero(t, p.UpdatedAt)
	require.NotNil(t, p.Config)
	require.True(t, p.Config.Equal(config))
	require.NotNil(t, p.Metadata)
}

func TestAddProcess(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	config := &app.Config{
		ID: "foobar",
	}

	err = s.addProcess(CommandAddProcess{
		Config: config,
	})
	require.Error(t, err)
	require.Empty(t, s.data.Process)

	config = &app.Config{
		ID:          "foobar",
		LimitCPU:    1,
		LimitMemory: 1,
	}

	err = s.addProcess(CommandAddProcess{
		Config: config,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.data.Process))

	config = &app.Config{
		ID:          "foobar",
		LimitCPU:    1,
		LimitMemory: 1,
	}

	err = s.addProcess(CommandAddProcess{
		Config: config,
	})
	require.Error(t, err)
	require.Equal(t, 1, len(s.data.Process))

	config = &app.Config{
		ID:          "foobar",
		Domain:      "barfoo",
		LimitCPU:    1,
		LimitMemory: 1,
	}

	err = s.addProcess(CommandAddProcess{
		Config: config,
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(s.data.Process))
}

func TestRemoveProcessCommand(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	config := &app.Config{
		ID:          "foobar",
		LimitCPU:    1,
		LimitMemory: 1,
	}

	err = s.applyCommand(Command{
		Operation: OpAddProcess,
		Data: CommandAddProcess{
			Config: config,
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, s.data.Process)

	err = s.applyCommand(Command{
		Operation: OpRemoveProcess,
		Data: CommandRemoveProcess{
			ID: config.ProcessID(),
		},
	})
	require.NoError(t, err)
	require.Empty(t, s.data.Process)

	err = s.applyCommand(Command{
		Operation: OpRemoveProcess,
		Data: CommandRemoveProcess{
			ID: config.ProcessID(),
		},
	})
	require.Error(t, err)
}

func TestRemoveProcess(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	config1 := &app.Config{
		ID:          "foobar",
		LimitCPU:    1,
		LimitMemory: 1,
	}

	config2 := &app.Config{
		ID:          "foobar",
		Domain:      "barfoo",
		LimitCPU:    1,
		LimitMemory: 1,
	}

	err = s.removeProcess(CommandRemoveProcess{
		ID: config1.ProcessID(),
	})
	require.Error(t, err)

	err = s.removeProcess(CommandRemoveProcess{
		ID: config2.ProcessID(),
	})
	require.Error(t, err)

	err = s.addProcess(CommandAddProcess{
		Config: config1,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.data.Process))

	err = s.addProcess(CommandAddProcess{
		Config: config2,
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(s.data.Process))

	err = s.removeProcess(CommandRemoveProcess{
		ID: config1.ProcessID(),
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.data.Process))

	err = s.removeProcess(CommandRemoveProcess{
		ID: config2.ProcessID(),
	})
	require.NoError(t, err)
	require.Empty(t, s.data.Process)
}

func TestUpdateProcessCommand(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	config := &app.Config{
		ID:          "foobar",
		LimitCPU:    1,
		LimitMemory: 1,
	}

	pid := config.ProcessID()

	err = s.applyCommand(Command{
		Operation: OpAddProcess,
		Data: CommandAddProcess{
			Config: config,
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, s.data.Process)

	config = &app.Config{
		ID:          "foobaz",
		LimitCPU:    1,
		LimitMemory: 1,
	}

	err = s.applyCommand(Command{
		Operation: OpUpdateProcess,
		Data: CommandUpdateProcess{
			ID:     pid,
			Config: config,
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, s.data.Process)
}

func TestUpdateProcess(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	config1 := &app.Config{
		ID:          "foobar",
		LimitCPU:    1,
		LimitMemory: 1,
	}

	config2 := &app.Config{
		ID:          "fooboz",
		LimitCPU:    1,
		LimitMemory: 1,
	}

	err = s.addProcess(CommandAddProcess{
		Config: config1,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.data.Process))

	err = s.addProcess(CommandAddProcess{
		Config: config2,
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(s.data.Process))

	config := &app.Config{
		ID:          "foobaz",
		LimitCPU:    1,
		LimitMemory: 1,
	}

	err = s.updateProcess(CommandUpdateProcess{
		ID:     config.ProcessID(),
		Config: config,
	})
	require.Error(t, err)

	config.ID = "fooboz"

	err = s.updateProcess(CommandUpdateProcess{
		ID:     config1.ProcessID(),
		Config: config,
	})
	require.Error(t, err)

	config.ID = "foobaz"
	config.LimitCPU = 0

	err = s.updateProcess(CommandUpdateProcess{
		ID:     config1.ProcessID(),
		Config: config,
	})
	require.Error(t, err)

	config.LimitCPU = 1

	err = s.updateProcess(CommandUpdateProcess{
		ID:     config1.ProcessID(),
		Config: config,
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(s.data.Process))

	err = s.updateProcess(CommandUpdateProcess{
		ID:     config.ProcessID(),
		Config: config,
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(s.data.Process))

	config3 := &app.Config{
		ID:          config.ID,
		LimitCPU:    1,
		LimitMemory: 2,
	}

	err = s.updateProcess(CommandUpdateProcess{
		ID:     config.ProcessID(),
		Config: config3,
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(s.data.Process))

	_, err = s.GetProcess(config1.ProcessID())
	require.Error(t, err)

	_, err = s.GetProcess(config2.ProcessID())
	require.NoError(t, err)

	_, err = s.GetProcess(config.ProcessID())
	require.NoError(t, err)
}

func TestSetProcessOrderCommand(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	config := &app.Config{
		ID:          "foobar",
		LimitCPU:    1,
		LimitMemory: 1,
	}

	err = s.applyCommand(Command{
		Operation: OpAddProcess,
		Data: CommandAddProcess{
			Config: config,
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, s.data.Process)

	p, err := s.GetProcess(config.ProcessID())
	require.NoError(t, err)
	require.Equal(t, "stop", p.Order)

	err = s.applyCommand(Command{
		Operation: OpSetProcessOrder,
		Data: CommandSetProcessOrder{
			ID:    config.ProcessID(),
			Order: "start",
		},
	})
	require.NoError(t, err)

	p, err = s.GetProcess(config.ProcessID())
	require.NoError(t, err)
	require.Equal(t, "start", p.Order)
}

func TestSetProcessOrder(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	config := &app.Config{
		ID:          "foobar",
		LimitCPU:    1,
		LimitMemory: 1,
	}

	err = s.setProcessOrder(CommandSetProcessOrder{
		ID:    config.ProcessID(),
		Order: "start",
	})
	require.Error(t, err)

	err = s.addProcess(CommandAddProcess{
		Config: config,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.data.Process))

	err = s.setProcessOrder(CommandSetProcessOrder{
		ID:    config.ProcessID(),
		Order: "start",
	})
	require.NoError(t, err)

	err = s.setProcessOrder(CommandSetProcessOrder{
		ID:    config.ProcessID(),
		Order: "stop",
	})
	require.NoError(t, err)

	p, err := s.GetProcess(config.ProcessID())
	require.NoError(t, err)
	require.Equal(t, "stop", p.Order)

	err = s.setProcessOrder(CommandSetProcessOrder{
		ID:    config.ProcessID(),
		Order: "start",
	})
	require.NoError(t, err)

	p, err = s.GetProcess(config.ProcessID())
	require.NoError(t, err)
	require.Equal(t, "start", p.Order)
}

func TestSetProcessMetadataCommand(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	config := &app.Config{
		ID:          "foobar",
		LimitCPU:    1,
		LimitMemory: 1,
	}

	err = s.applyCommand(Command{
		Operation: OpAddProcess,
		Data: CommandAddProcess{
			Config: config,
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, s.data.Process)

	p, err := s.GetProcess(config.ProcessID())
	require.NoError(t, err)
	require.Empty(t, p.Metadata)

	metadata := "bar"

	err = s.applyCommand(Command{
		Operation: OpSetProcessMetadata,
		Data: CommandSetProcessMetadata{
			ID:   config.ProcessID(),
			Key:  "foo",
			Data: metadata,
		},
	})
	require.NoError(t, err)

	p, err = s.GetProcess(config.ProcessID())
	require.NoError(t, err)

	require.NotEmpty(t, p.Metadata)
	require.Equal(t, 1, len(p.Metadata))
	require.Equal(t, "bar", p.Metadata["foo"])
}

func TestSetProcessMetadata(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	config := &app.Config{
		ID:          "foobar",
		LimitCPU:    1,
		LimitMemory: 1,
	}

	err = s.setProcessMetadata(CommandSetProcessMetadata{
		ID:   config.ProcessID(),
		Key:  "foo",
		Data: "bar",
	})
	require.Error(t, err)

	err = s.addProcess(CommandAddProcess{
		Config: config,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.data.Process))

	err = s.setProcessMetadata(CommandSetProcessMetadata{
		ID:   config.ProcessID(),
		Key:  "foo",
		Data: "bar",
	})
	require.NoError(t, err)

	err = s.setProcessMetadata(CommandSetProcessMetadata{
		ID:   config.ProcessID(),
		Key:  "faa",
		Data: "boz",
	})
	require.NoError(t, err)

	p, err := s.GetProcess(config.ProcessID())
	require.NoError(t, err)

	require.NotEmpty(t, p.Metadata)
	require.Equal(t, 2, len(p.Metadata))
	require.Equal(t, "bar", p.Metadata["foo"])
	require.Equal(t, "boz", p.Metadata["faa"])

	err = s.setProcessMetadata(CommandSetProcessMetadata{
		ID:   config.ProcessID(),
		Key:  "faa",
		Data: nil,
	})
	require.NoError(t, err)

	p, err = s.GetProcess(config.ProcessID())
	require.NoError(t, err)

	require.NotEmpty(t, p.Metadata)
	require.Equal(t, 1, len(p.Metadata))
	require.Equal(t, "bar", p.Metadata["foo"])

	err = s.setProcessMetadata(CommandSetProcessMetadata{
		ID:   config.ProcessID(),
		Key:  "foo",
		Data: "bor",
	})
	require.NoError(t, err)

	p, err = s.GetProcess(config.ProcessID())
	require.NoError(t, err)

	require.NotEmpty(t, p.Metadata)
	require.Equal(t, 1, len(p.Metadata))
	require.Equal(t, "bor", p.Metadata["foo"])
}

func TestSetProcessErrorCommand(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	config := &app.Config{
		ID:          "foobar",
		LimitCPU:    1,
		LimitMemory: 1,
	}

	err = s.applyCommand(Command{
		Operation: OpAddProcess,
		Data: CommandAddProcess{
			Config: config,
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, s.data.Process)

	p, err := s.GetProcess(config.ProcessID())
	require.NoError(t, err)
	require.Equal(t, "", p.Error)

	err = s.applyCommand(Command{
		Operation: OpSetProcessError,
		Data: CommandSetProcessError{
			ID:    config.ProcessID(),
			Error: "foobar",
		},
	})
	require.NoError(t, err)

	p, err = s.GetProcess(config.ProcessID())
	require.NoError(t, err)
	require.Equal(t, "foobar", p.Error)
}

func TestSetProcessError(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	config := &app.Config{
		ID:          "foobar",
		LimitCPU:    1,
		LimitMemory: 1,
	}

	err = s.setProcessError(CommandSetProcessError{
		ID:    config.ProcessID(),
		Error: "foobar",
	})
	require.Error(t, err)

	err = s.addProcess(CommandAddProcess{
		Config: config,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.data.Process))

	err = s.setProcessError(CommandSetProcessError{
		ID:    config.ProcessID(),
		Error: "foobar",
	})
	require.NoError(t, err)

	err = s.setProcessError(CommandSetProcessError{
		ID:    config.ProcessID(),
		Error: "",
	})
	require.NoError(t, err)

	p, err := s.GetProcess(config.ProcessID())
	require.NoError(t, err)
	require.Equal(t, "", p.Error)

	err = s.setProcessError(CommandSetProcessError{
		ID:    config.ProcessID(),
		Error: "foobar",
	})
	require.NoError(t, err)

	p, err = s.GetProcess(config.ProcessID())
	require.NoError(t, err)
	require.Equal(t, "foobar", p.Error)
}

func TestAddIdentityCommand(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	identity := identity.User{
		Name: "foobar",
	}

	err = s.applyCommand(Command{
		Operation: OpAddIdentity,
		Data: CommandAddIdentity{
			Identity: identity,
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.data.Users.Users))
}

func TestAddIdentity(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	identity := identity.User{
		Name: "foobar",
	}

	err = s.addIdentity(CommandAddIdentity{
		Identity: identity,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.data.Users.Users))
	require.Equal(t, 0, len(s.data.Policies.Policies))

	err = s.addIdentity(CommandAddIdentity{
		Identity: identity,
	})
	require.Error(t, err)
	require.Equal(t, 1, len(s.data.Users.Users))
	require.Equal(t, 0, len(s.data.Policies.Policies))
}

func TestRemoveIdentityCommand(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	identity := identity.User{
		Name: "foobar",
	}

	err = s.applyCommand(Command{
		Operation: OpAddIdentity,
		Data: CommandAddIdentity{
			Identity: identity,
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.data.Users.Users))

	err = s.applyCommand(Command{
		Operation: OpRemoveIdentity,
		Data: CommandRemoveIdentity{
			Name: "foobar",
		},
	})
	require.NoError(t, err)
	require.Equal(t, 0, len(s.data.Users.Users))
}

func TestRemoveIdentity(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	identity := identity.User{
		Name: "foobar",
	}

	err = s.addIdentity(CommandAddIdentity{
		Identity: identity,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.data.Users.Users))
	require.Equal(t, 0, len(s.data.Policies.Policies))

	err = s.removeIdentity(CommandRemoveIdentity{
		Name: "foobar",
	})
	require.NoError(t, err)
	require.Equal(t, 0, len(s.data.Users.Users))
	require.Equal(t, 0, len(s.data.Policies.Policies))

	err = s.removeIdentity(CommandRemoveIdentity{
		Name: "foobar",
	})
	require.NoError(t, err)
	require.Equal(t, 0, len(s.data.Users.Users))
	require.Equal(t, 0, len(s.data.Policies.Policies))
}

func TestSetPoliciesCommand(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	identity := identity.User{
		Name: "foobar",
	}

	err = s.applyCommand(Command{
		Operation: OpAddIdentity,
		Data: CommandAddIdentity{
			Identity: identity,
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.data.Users.Users))
	require.Equal(t, 0, len(s.data.Policies.Policies))

	err = s.applyCommand(Command{
		Operation: OpSetPolicies,
		Data: CommandSetPolicies{
			Name: "foobar",
			Policies: []access.Policy{
				{
					Name:     "bla",
					Domain:   "bla",
					Resource: "bla",
					Actions:  []string{},
				},
				{
					Name:     "foo",
					Domain:   "foo",
					Resource: "foo",
					Actions:  []string{},
				},
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.data.Users.Users))
	require.Equal(t, 2, len(s.data.Policies.Policies["foobar"]))
}

func TestSetPolicies(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	identity := identity.User{
		Name: "foobar",
	}

	policies := []access.Policy{
		{
			Name:     "bla",
			Domain:   "bla",
			Resource: "bla",
			Actions:  []string{},
		},
		{
			Name:     "foo",
			Domain:   "foo",
			Resource: "foo",
			Actions:  []string{},
		},
	}

	err = s.setPolicies(CommandSetPolicies{
		Name:     "foobar",
		Policies: policies,
	})
	require.Error(t, err)
	require.Equal(t, 0, len(s.data.Policies.Policies["foobar"]))

	err = s.addIdentity(CommandAddIdentity{
		Identity: identity,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.data.Users.Users))
	require.Equal(t, 0, len(s.data.Policies.Policies))

	err = s.setPolicies(CommandSetPolicies{
		Name:     "foobar",
		Policies: policies,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.data.Users.Users))
	require.Equal(t, 2, len(s.data.Policies.Policies["foobar"]))
}

func TestUpdateUserCommand(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	idty1 := identity.User{
		Name: "foobar1",
	}

	idty2 := identity.User{
		Name: "foobar2",
	}

	err = s.applyCommand(Command{
		Operation: OpAddIdentity,
		Data: CommandAddIdentity{
			Identity: idty1,
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.data.Users.Users))

	err = s.applyCommand(Command{
		Operation: OpAddIdentity,
		Data: CommandAddIdentity{
			Identity: idty2,
		},
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(s.data.Users.Users))

	err = s.applyCommand(Command{
		Operation: OpUpdateIdentity,
		Data: CommandUpdateIdentity{
			Name: "foobar1",
			Identity: identity.User{
				Name: "foobar3",
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(s.data.Users.Users))
}

func TestUpdateIdentity(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	idty1 := identity.User{
		Name: "foobar1",
	}

	idty2 := identity.User{
		Name: "foobar2",
	}

	err = s.addIdentity(CommandAddIdentity{
		Identity: idty1,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.data.Users.Users))

	err = s.addIdentity(CommandAddIdentity{
		Identity: idty2,
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(s.data.Users.Users))

	idty := identity.User{
		Name: "foobaz",
	}

	err = s.updateIdentity(CommandUpdateIdentity{
		Name:     "foobar",
		Identity: idty,
	})
	require.Error(t, err)
	require.Equal(t, 2, len(s.data.Users.Users))

	idty.Name = "foobar2"

	err = s.updateIdentity(CommandUpdateIdentity{
		Name:     "foobar1",
		Identity: idty,
	})
	require.Error(t, err)
	require.Equal(t, 2, len(s.data.Users.Users))

	idty.Name = "foobaz"

	err = s.updateIdentity(CommandUpdateIdentity{
		Name:     "foobar1",
		Identity: idty,
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(s.data.Users.Users))

	u := s.GetUser("foobar1")
	require.Empty(t, u.Users)

	u = s.GetUser("foobar2")
	require.NotEmpty(t, u.Users)

	u = s.GetUser("foobaz")
	require.NotEmpty(t, u.Users)
}

func TestUpdateIdentityWithPolicies(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	idty1 := identity.User{
		Name: "foobar",
	}

	policies := []access.Policy{
		{
			Name:     "bla",
			Domain:   "bla",
			Resource: "bla",
			Actions:  []string{},
		},
		{
			Name:     "foo",
			Domain:   "foo",
			Resource: "foo",
			Actions:  []string{},
		},
	}

	err = s.addIdentity(CommandAddIdentity{
		Identity: idty1,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.data.Users.Users))

	err = s.setPolicies(CommandSetPolicies{
		Name:     "foobar",
		Policies: policies,
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(s.data.Policies.Policies["foobar"]))

	err = s.updateIdentity(CommandUpdateIdentity{
		Name:     "foobar",
		Identity: idty1,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.data.Users.Users))
	require.Equal(t, 2, len(s.data.Policies.Policies["foobar"]))

	idty2 := identity.User{
		Name: "foobaz",
	}

	err = s.updateIdentity(CommandUpdateIdentity{
		Name:     "foobar",
		Identity: idty2,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.data.Users.Users))
	require.Equal(t, 0, len(s.data.Policies.Policies["foobar"]))
	require.Equal(t, 2, len(s.data.Policies.Policies["foobaz"]))
}

func TestSetProcessNodeMapCommand(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	m1 := map[string]string{
		"key": "value1",
	}

	err = s.applyCommand(Command{
		Operation: OpSetProcessNodeMap,
		Data: CommandSetProcessNodeMap{
			Map: m1,
		},
	})
	require.NoError(t, err)
	require.Equal(t, m1, s.data.ProcessNodeMap)
}

func TestSetProcessNodeMap(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	m1 := map[string]string{
		"key": "value1",
	}

	err = s.setProcessNodeMap(CommandSetProcessNodeMap{
		Map: m1,
	})
	require.NoError(t, err)
	require.Equal(t, m1, s.data.ProcessNodeMap)

	m2 := map[string]string{
		"key": "value2",
	}

	err = s.setProcessNodeMap(CommandSetProcessNodeMap{
		Map: m2,
	})
	require.NoError(t, err)
	require.Equal(t, m2, s.data.ProcessNodeMap)

	m := s.GetProcessNodeMap()
	require.Equal(t, m2, m)
}

func TestCreateLockCommand(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	err = s.applyCommand(Command{
		Operation: OpCreateLock,
		Data: CommandCreateLock{
			Name:       "foobar",
			ValidUntil: time.Now().Add(3 * time.Second),
		},
	})
	require.NoError(t, err)

	_, ok := s.data.Locks["foobar"]
	require.True(t, ok)
}

func TestCreateLock(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	cmd := CommandCreateLock{
		Name:       "foobar",
		ValidUntil: time.Now().Add(3 * time.Second),
	}

	err = s.createLock(cmd)
	require.NoError(t, err)

	err = s.createLock(cmd)
	require.Error(t, err)

	require.Eventually(t, func() bool {
		err = s.createLock(cmd)
		return err == nil
	}, 5*time.Second, time.Second)
}

func TestDeleteLockCommand(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	err = s.applyCommand(Command{
		Operation: OpCreateLock,
		Data: CommandCreateLock{
			Name:       "foobar",
			ValidUntil: time.Now().Add(10 * time.Second),
		},
	})
	require.NoError(t, err)

	_, ok := s.data.Locks["foobar"]
	require.True(t, ok)

	err = s.applyCommand(Command{
		Operation: OpDeleteLock,
		Data: CommandDeleteLock{
			Name: "foobar",
		},
	})
	require.NoError(t, err)

	_, ok = s.data.Locks["foobar"]
	require.False(t, ok)
}

func TestDeleteLock(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	err = s.deleteLock(CommandDeleteLock{
		Name: "foobar",
	})
	require.NoError(t, err)

	cmd := CommandCreateLock{
		Name:       "foobar",
		ValidUntil: time.Now().Add(10 * time.Second),
	}

	err = s.createLock(cmd)
	require.NoError(t, err)

	err = s.createLock(cmd)
	require.Error(t, err)

	err = s.deleteLock(CommandDeleteLock{
		Name: "foobar",
	})
	require.NoError(t, err)

	err = s.createLock(cmd)
	require.NoError(t, err)
}

func TestClearLocksCommand(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	err = s.applyCommand(Command{
		Operation: OpCreateLock,
		Data: CommandCreateLock{
			Name:       "foobar",
			ValidUntil: time.Now().Add(3 * time.Second),
		},
	})
	require.NoError(t, err)

	_, ok := s.data.Locks["foobar"]
	require.True(t, ok)

	err = s.applyCommand(Command{
		Operation: OpClearLocks,
		Data:      CommandClearLocks{},
	})
	require.NoError(t, err)

	_, ok = s.data.Locks["foobar"]
	require.True(t, ok)

	time.Sleep(3 * time.Second)

	err = s.applyCommand(Command{
		Operation: OpClearLocks,
		Data:      CommandClearLocks{},
	})
	require.NoError(t, err)

	_, ok = s.data.Locks["foobar"]
	require.False(t, ok)
}

func TestClearLocks(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	cmd := CommandCreateLock{
		Name:       "foobar",
		ValidUntil: time.Now().Add(3 * time.Second),
	}

	err = s.createLock(cmd)
	require.NoError(t, err)

	err = s.clearLocks(CommandClearLocks{})
	require.NoError(t, err)

	err = s.createLock(cmd)
	require.Error(t, err)

	time.Sleep(3 * time.Second)

	err = s.clearLocks(CommandClearLocks{})
	require.NoError(t, err)

	err = s.createLock(cmd)
	require.NoError(t, err)
}

func TestSetKVCommand(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	err = s.applyCommand(Command{
		Operation: OpSetKV,
		Data: CommandSetKV{
			Key:   "foo",
			Value: "bar",
		},
	})
	require.NoError(t, err)

	_, ok := s.data.KVS["foo"]
	require.True(t, ok)
}

func TestSetKV(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	err = s.setKV(CommandSetKV{
		Key:   "foo",
		Value: "bar",
	})
	require.NoError(t, err)

	value, err := s.GetFromKVS("foo")
	require.NoError(t, err)
	require.Equal(t, "bar", value.Value)

	updatedAt := value.UpdatedAt

	err = s.setKV(CommandSetKV{
		Key:   "foo",
		Value: "baz",
	})
	require.NoError(t, err)

	value, err = s.GetFromKVS("foo")
	require.NoError(t, err)
	require.Equal(t, "baz", value.Value)
	require.Greater(t, value.UpdatedAt, updatedAt)
}

func TestUnsetKVCommand(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	err = s.applyCommand(Command{
		Operation: OpSetKV,
		Data: CommandSetKV{
			Key:   "foo",
			Value: "bar",
		},
	})
	require.NoError(t, err)

	_, ok := s.data.KVS["foo"]
	require.True(t, ok)

	err = s.applyCommand(Command{
		Operation: OpUnsetKV,
		Data: CommandUnsetKV{
			Key: "foo",
		},
	})
	require.NoError(t, err)

	_, ok = s.data.KVS["foo"]
	require.False(t, ok)

	err = s.applyCommand(Command{
		Operation: OpUnsetKV,
		Data: CommandUnsetKV{
			Key: "foo",
		},
	})
	require.Error(t, err)
	require.Equal(t, fs.ErrNotExist, err)
}

func TestUnsetKV(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	err = s.setKV(CommandSetKV{
		Key:   "foo",
		Value: "bar",
	})
	require.NoError(t, err)

	_, err = s.GetFromKVS("foo")
	require.NoError(t, err)

	err = s.unsetKV(CommandUnsetKV{
		Key: "foo",
	})
	require.NoError(t, err)

	_, err = s.GetFromKVS("foo")
	require.Error(t, err)
	require.Equal(t, fs.ErrNotExist, err)
}

func TestApplyCommand(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	err = s.applyCommand(Command{
		Operation: "unknown",
		Data:      nil,
	})
	require.Error(t, err)
}

func TestApply(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	entry := &raft.Log{
		Index: 1,
		Term:  1,
		Type:  raft.LogCommand,
		Data:  []byte("123"),
	}

	res := s.Apply(entry)
	require.NotNil(t, res)

	cmd := Command{
		Operation: "unknown",
		Data:      nil,
	}

	data, err := json.Marshal(&cmd)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	entry.Data = data

	res = s.Apply(entry)
	require.NotNil(t, res)

	cmd = Command{
		Operation: OpSetProcessNodeMap,
		Data: CommandSetProcessNodeMap{
			Map: nil,
		},
	}

	data, err = json.Marshal(&cmd)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	entry.Data = data

	res = s.Apply(entry)
	require.Nil(t, res)
}

func TestApplyWithCallback(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	var op Operation

	s.OnApply(func(o Operation) {
		op = o
	})

	cmd := Command{
		Operation: OpSetProcessNodeMap,
		Data: CommandSetProcessNodeMap{
			Map: nil,
		},
	}

	data, err := json.Marshal(&cmd)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	entry := &raft.Log{
		Index: 1,
		Term:  1,
		Type:  raft.LogCommand,
		Data:  data,
	}

	res := s.Apply(entry)
	require.Nil(t, res)

	require.Equal(t, OpSetProcessNodeMap, op)
}
