package store

import (
	"encoding/json"
	"testing"

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

	require.NotNil(t, s.Process)
	require.NotNil(t, s.ProcessNodeMap)
	require.NotNil(t, s.Users.Users)
	require.NotNil(t, s.Policies.Policies)
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
	require.NotEmpty(t, s.Process)

	p, ok := s.Process["foobar@"]
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
	require.Empty(t, s.Process)

	config = &app.Config{
		ID:          "foobar",
		LimitCPU:    1,
		LimitMemory: 1,
	}

	err = s.addProcess(CommandAddProcess{
		Config: config,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.Process))

	config = &app.Config{
		ID:          "foobar",
		LimitCPU:    1,
		LimitMemory: 1,
	}

	err = s.addProcess(CommandAddProcess{
		Config: config,
	})
	require.Error(t, err)
	require.Equal(t, 1, len(s.Process))

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
	require.Equal(t, 2, len(s.Process))
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
	require.NotEmpty(t, s.Process)

	err = s.applyCommand(Command{
		Operation: OpRemoveProcess,
		Data: CommandRemoveProcess{
			ID: config.ProcessID(),
		},
	})
	require.NoError(t, err)
	require.Empty(t, s.Process)

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
	require.Equal(t, 1, len(s.Process))

	err = s.addProcess(CommandAddProcess{
		Config: config2,
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(s.Process))

	err = s.removeProcess(CommandRemoveProcess{
		ID: config1.ProcessID(),
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.Process))

	err = s.removeProcess(CommandRemoveProcess{
		ID: config2.ProcessID(),
	})
	require.NoError(t, err)
	require.Empty(t, s.Process)
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
	require.NotEmpty(t, s.Process)

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
	require.NotEmpty(t, s.Process)
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
	require.Equal(t, 1, len(s.Process))

	err = s.addProcess(CommandAddProcess{
		Config: config2,
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(s.Process))

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
	require.Equal(t, 2, len(s.Process))

	err = s.updateProcess(CommandUpdateProcess{
		ID:     config.ProcessID(),
		Config: config,
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(s.Process))

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
	require.Equal(t, 2, len(s.Process))

	_, err = s.GetProcess(config1.ProcessID())
	require.Error(t, err)

	_, err = s.GetProcess(config2.ProcessID())
	require.NoError(t, err)

	_, err = s.GetProcess(config.ProcessID())
	require.NoError(t, err)
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
	require.NotEmpty(t, s.Process)

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
	require.Equal(t, 1, len(s.Process))

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
	require.Equal(t, 1, len(s.Users.Users))
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
	require.Equal(t, 1, len(s.Users.Users))
	require.Equal(t, 0, len(s.Policies.Policies))

	err = s.addIdentity(CommandAddIdentity{
		Identity: identity,
	})
	require.Error(t, err)
	require.Equal(t, 1, len(s.Users.Users))
	require.Equal(t, 0, len(s.Policies.Policies))
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
	require.Equal(t, 1, len(s.Users.Users))

	err = s.applyCommand(Command{
		Operation: OpRemoveIdentity,
		Data: CommandRemoveIdentity{
			Name: "foobar",
		},
	})
	require.NoError(t, err)
	require.Equal(t, 0, len(s.Users.Users))
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
	require.Equal(t, 1, len(s.Users.Users))
	require.Equal(t, 0, len(s.Policies.Policies))

	err = s.removeIdentity(CommandRemoveIdentity{
		Name: "foobar",
	})
	require.NoError(t, err)
	require.Equal(t, 0, len(s.Users.Users))
	require.Equal(t, 0, len(s.Policies.Policies))

	err = s.removeIdentity(CommandRemoveIdentity{
		Name: "foobar",
	})
	require.NoError(t, err)
	require.Equal(t, 0, len(s.Users.Users))
	require.Equal(t, 0, len(s.Policies.Policies))
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
	require.Equal(t, 1, len(s.Users.Users))
	require.Equal(t, 0, len(s.Policies.Policies))

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
	require.Equal(t, 1, len(s.Users.Users))
	require.Equal(t, 2, len(s.Policies.Policies["foobar"]))
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
	require.Equal(t, 0, len(s.Policies.Policies["foobar"]))

	err = s.addIdentity(CommandAddIdentity{
		Identity: identity,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.Users.Users))
	require.Equal(t, 0, len(s.Policies.Policies))

	err = s.setPolicies(CommandSetPolicies{
		Name:     "foobar",
		Policies: policies,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.Users.Users))
	require.Equal(t, 2, len(s.Policies.Policies["foobar"]))
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
	require.Equal(t, 1, len(s.Users.Users))

	err = s.applyCommand(Command{
		Operation: OpAddIdentity,
		Data: CommandAddIdentity{
			Identity: idty2,
		},
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(s.Users.Users))

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
	require.Equal(t, 2, len(s.Users.Users))
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
	require.Equal(t, 1, len(s.Users.Users))

	err = s.addIdentity(CommandAddIdentity{
		Identity: idty2,
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(s.Users.Users))

	idty := identity.User{
		Name: "foobaz",
	}

	err = s.updateIdentity(CommandUpdateIdentity{
		Name:     "foobar",
		Identity: idty,
	})
	require.Error(t, err)
	require.Equal(t, 2, len(s.Users.Users))

	idty.Name = "foobar2"

	err = s.updateIdentity(CommandUpdateIdentity{
		Name:     "foobar1",
		Identity: idty,
	})
	require.Error(t, err)
	require.Equal(t, 2, len(s.Users.Users))

	idty.Name = "foobaz"

	err = s.updateIdentity(CommandUpdateIdentity{
		Name:     "foobar1",
		Identity: idty,
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(s.Users.Users))

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
	require.Equal(t, 1, len(s.Users.Users))

	err = s.setPolicies(CommandSetPolicies{
		Name:     "foobar",
		Policies: policies,
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(s.Policies.Policies["foobar"]))

	err = s.updateIdentity(CommandUpdateIdentity{
		Name:     "foobar",
		Identity: idty1,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.Users.Users))
	require.Equal(t, 2, len(s.Policies.Policies["foobar"]))

	idty2 := identity.User{
		Name: "foobaz",
	}

	err = s.updateIdentity(CommandUpdateIdentity{
		Name:     "foobar",
		Identity: idty2,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.Users.Users))
	require.Equal(t, 0, len(s.Policies.Policies["foobar"]))
	require.Equal(t, 2, len(s.Policies.Policies["foobaz"]))
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
	require.Equal(t, m1, s.ProcessNodeMap)
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
	require.Equal(t, m1, s.ProcessNodeMap)

	m2 := map[string]string{
		"key": "value2",
	}

	err = s.setProcessNodeMap(CommandSetProcessNodeMap{
		Map: m2,
	})
	require.NoError(t, err)
	require.Equal(t, m2, s.ProcessNodeMap)

	m := s.GetProcessNodeMap()
	require.Equal(t, m2, m)
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
