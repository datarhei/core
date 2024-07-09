package store

import (
	"testing"

	"github.com/datarhei/core/v16/restream/app"
	"github.com/stretchr/testify/require"
)

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

	_, err = s.ProcessGet(config1.ProcessID())
	require.Error(t, err)

	_, err = s.ProcessGet(config2.ProcessID())
	require.NoError(t, err)

	_, err = s.ProcessGet(config.ProcessID())
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

	p, err := s.ProcessGet(config.ProcessID())
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

	p, err = s.ProcessGet(config.ProcessID())
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

	p, err := s.ProcessGet(config.ProcessID())
	require.NoError(t, err)
	require.Equal(t, "stop", p.Order)

	err = s.setProcessOrder(CommandSetProcessOrder{
		ID:    config.ProcessID(),
		Order: "start",
	})
	require.NoError(t, err)

	p, err = s.ProcessGet(config.ProcessID())
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

	p, err := s.ProcessGet(config.ProcessID())
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

	p, err = s.ProcessGet(config.ProcessID())
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

	p, err := s.ProcessGet(config.ProcessID())
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

	p, err = s.ProcessGet(config.ProcessID())
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

	p, err = s.ProcessGet(config.ProcessID())
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

	p, err := s.ProcessGet(config.ProcessID())
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

	p, err = s.ProcessGet(config.ProcessID())
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

	p, err := s.ProcessGet(config.ProcessID())
	require.NoError(t, err)
	require.Equal(t, "", p.Error)

	err = s.setProcessError(CommandSetProcessError{
		ID:    config.ProcessID(),
		Error: "foobar",
	})
	require.NoError(t, err)

	p, err = s.ProcessGet(config.ProcessID())
	require.NoError(t, err)
	require.Equal(t, "foobar", p.Error)
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

	m := s.ProcessGetNodeMap()
	require.Equal(t, m2, m)
}
