package log

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoglevelNames(t *testing.T) {
	assert.Equal(t, "DEBUG", Ldebug.String())
	assert.Equal(t, "ERROR", Lerror.String())
	assert.Equal(t, "WARN", Lwarn.String())
	assert.Equal(t, "INFO", Linfo.String())
	assert.Equal(t, `SILENT`, Lsilent.String())
}

func TestLogColorToNotTTY(t *testing.T) {
	var buffer bytes.Buffer
	writer := bufio.NewWriter(&buffer)

	w := NewConsoleWriter(writer, Linfo, true).(*syncWriter)
	formatter := w.writer.(*consoleWriter).formatter.(*consoleFormatter)

	assert.NotEqual(t, true, formatter.color, "Color should not be used on a buffer logger")
}

func TestLogContext(t *testing.T) {

	var buffer bytes.Buffer
	writer := bufio.NewWriter(&buffer)

	logger := New("component").WithOutput(NewConsoleWriter(writer, Ldebug, false))

	logger.Debug().Log("debug")
	logger.Info().Log("info")
	logger.Warn().Log("warn")
	logger.Error().Log("error")
	writer.Flush()

	lenWithCtx := buffer.Len()
	buffer.Reset()

	logger = logger.WithComponent("")

	logger.Debug().Log("debug")
	logger.Info().Log("info")
	logger.Warn().Log("warn")
	logger.Error().Log("error")
	writer.Flush()

	lenWithoutCtx := buffer.Len()
	buffer.Reset()

	assert.Greater(t, lenWithCtx, lenWithoutCtx, "Log line length without context is not shorter than with context")
}

func TestLogClone(t *testing.T) {
	var buffer bytes.Buffer
	writer := bufio.NewWriter(&buffer)

	logger := New("test").WithOutput(NewConsoleWriter(writer, Linfo, false))

	logger.Info().Log("info")
	writer.Flush()

	assert.Contains(t, buffer.String(), `component="test"`)

	buffer.Reset()

	logger2 := logger.WithComponent("tset")

	logger2.Info().Log("info")
	writer.Flush()

	assert.Contains(t, buffer.String(), `component="tset"`)
}

func TestLogSilent(t *testing.T) {
	var buffer bytes.Buffer
	writer := bufio.NewWriter(&buffer)

	logger := New("test").WithOutput(NewConsoleWriter(writer, Lsilent, false))

	logger.Debug().Log("debug")
	writer.Flush()
	assert.Equal(t, 0, buffer.Len(), "Buffer should be empty")
	buffer.Reset()

	logger.Info().Log("info")
	writer.Flush()
	assert.Equal(t, 0, buffer.Len(), "Buffer should be empty")
	buffer.Reset()

	logger.Warn().Log("warn")
	writer.Flush()
	assert.Equal(t, 0, buffer.Len(), "Buffer should be empty")
	buffer.Reset()

	logger.Error().Log("error")
	writer.Flush()
	assert.Equal(t, 0, buffer.Len(), "Buffer should be empty")
	buffer.Reset()
}

func TestLogDebug(t *testing.T) {
	var buffer bytes.Buffer
	writer := bufio.NewWriter(&buffer)

	logger := New("test").WithOutput(NewConsoleWriter(writer, Ldebug, false))

	logger.Debug().Log("debug")
	writer.Flush()
	assert.NotEqual(t, 0, buffer.Len(), "Buffer should not be empty")
	buffer.Reset()

	logger.Info().Log("info")
	writer.Flush()
	assert.NotEqual(t, 0, buffer.Len(), "Buffer should not be empty")
	buffer.Reset()

	logger.Warn().Log("warn")
	writer.Flush()
	assert.NotEqual(t, 0, buffer.Len(), "Buffer should not be empty")
	buffer.Reset()

	logger.Error().Log("error")
	writer.Flush()
	assert.NotEqual(t, 0, buffer.Len(), "Buffer should not be empty")
	buffer.Reset()
}

func TestLogInfo(t *testing.T) {
	var buffer bytes.Buffer
	writer := bufio.NewWriter(&buffer)

	logger := New("test").WithOutput(NewConsoleWriter(writer, Linfo, false))

	logger.Debug().Log("debug")
	writer.Flush()
	assert.Equal(t, 0, buffer.Len(), "Buffer should be empty")
	buffer.Reset()

	logger.Info().Log("info")
	writer.Flush()
	assert.NotEqual(t, 0, buffer.Len(), "Buffer should not be empty")
	buffer.Reset()

	logger.Warn().Log("warn")
	writer.Flush()
	assert.NotEqual(t, 0, buffer.Len(), "Buffer should not be empty")
	buffer.Reset()

	logger.Error().Log("error")
	writer.Flush()
	assert.NotEqual(t, 0, buffer.Len(), "Buffer should not be empty")
	buffer.Reset()
}

func TestLogWarn(t *testing.T) {
	var buffer bytes.Buffer
	writer := bufio.NewWriter(&buffer)

	logger := New("test").WithOutput(NewConsoleWriter(writer, Lwarn, false))

	logger.Debug().Log("debug")
	writer.Flush()
	assert.Equal(t, 0, buffer.Len(), "Buffer should be empty")
	buffer.Reset()

	logger.Info().Log("info")
	writer.Flush()
	assert.Equal(t, 0, buffer.Len(), "Buffer should be empty")
	buffer.Reset()

	logger.Warn().Log("warn")
	writer.Flush()
	assert.NotEqual(t, 0, buffer.Len(), "Buffer should not be empty")
	buffer.Reset()

	logger.Error().Log("error")
	writer.Flush()
	assert.NotEqual(t, 0, buffer.Len(), "Buffer should not be empty")
	buffer.Reset()
}

func TestLogError(t *testing.T) {
	var buffer bytes.Buffer
	writer := bufio.NewWriter(&buffer)

	logger := New("test").WithOutput(NewConsoleWriter(writer, Lerror, false))

	logger.Debug().Log("debug")
	writer.Flush()
	assert.Equal(t, 0, buffer.Len(), "Buffer should be empty")
	buffer.Reset()

	logger.Info().Log("info")
	writer.Flush()
	assert.Equal(t, 0, buffer.Len(), "Buffer should be empty")
	buffer.Reset()

	logger.Warn().Log("warn")
	writer.Flush()
	assert.Equal(t, 0, buffer.Len(), "Buffer should be empty")
	buffer.Reset()

	logger.Error().Log("error")
	writer.Flush()
	assert.NotEqual(t, 0, buffer.Len(), "Buffer should not be empty")
	buffer.Reset()
}
