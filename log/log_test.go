package log

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoglevelNames(t *testing.T) {
	require.Equal(t, "DEBUG", Ldebug.String())
	require.Equal(t, "ERROR", Lerror.String())
	require.Equal(t, "WARN", Lwarn.String())
	require.Equal(t, "INFO", Linfo.String())
	require.Equal(t, `SILENT`, Lsilent.String())
}

func TestLogColorToNotTTY(t *testing.T) {
	var buffer bytes.Buffer
	writer := bufio.NewWriter(&buffer)

	w := NewLevelWriter(NewConsoleWriter(writer, true), Linfo).(*levelWriter).writer.(*syncWriter)
	formatter := w.writer.(*consoleWriter).formatter.(*consoleFormatter)

	require.NotEqual(t, true, formatter.color, "Color should not be used on a buffer logger")
}

func TestLogContext(t *testing.T) {

	var buffer bytes.Buffer
	writer := bufio.NewWriter(&buffer)

	logger := New("component").WithOutput(NewLevelWriter(NewConsoleWriter(writer, false), Ldebug))

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

	require.Greater(t, lenWithCtx, lenWithoutCtx, "Log line length without context is not shorter than with context")
}

func TestLogClone(t *testing.T) {
	var buffer bytes.Buffer
	writer := bufio.NewWriter(&buffer)

	logger := New("test").WithOutput(NewLevelWriter(NewConsoleWriter(writer, false), Linfo))

	logger.Info().Log("info")
	writer.Flush()

	require.Contains(t, buffer.String(), `component="test"`)

	buffer.Reset()

	logger2 := logger.WithComponent("tset")

	logger2.Info().Log("info")
	writer.Flush()

	require.Contains(t, buffer.String(), `component="tset"`)
}

func TestLogSilent(t *testing.T) {
	var buffer bytes.Buffer
	writer := bufio.NewWriter(&buffer)

	logger := New("test").WithOutput(NewLevelWriter(NewConsoleWriter(writer, false), Lsilent))

	logger.Debug().Log("debug")
	writer.Flush()
	require.Equal(t, 0, buffer.Len(), "Buffer should be empty")
	buffer.Reset()

	logger.Info().Log("info")
	writer.Flush()
	require.Equal(t, 0, buffer.Len(), "Buffer should be empty")
	buffer.Reset()

	logger.Warn().Log("warn")
	writer.Flush()
	require.Equal(t, 0, buffer.Len(), "Buffer should be empty")
	buffer.Reset()

	logger.Error().Log("error")
	writer.Flush()
	require.Equal(t, 0, buffer.Len(), "Buffer should be empty")
	buffer.Reset()
}

func TestLogDebug(t *testing.T) {
	var buffer bytes.Buffer
	writer := bufio.NewWriter(&buffer)

	logger := New("test").WithOutput(NewLevelWriter(NewConsoleWriter(writer, false), Ldebug))

	logger.Debug().Log("debug")
	writer.Flush()
	require.NotEqual(t, 0, buffer.Len(), "Buffer should not be empty")
	buffer.Reset()

	logger.Info().Log("info")
	writer.Flush()
	require.NotEqual(t, 0, buffer.Len(), "Buffer should not be empty")
	buffer.Reset()

	logger.Warn().Log("warn")
	writer.Flush()
	require.NotEqual(t, 0, buffer.Len(), "Buffer should not be empty")
	buffer.Reset()

	logger.Error().Log("error")
	writer.Flush()
	require.NotEqual(t, 0, buffer.Len(), "Buffer should not be empty")
	buffer.Reset()
}

func TestLogInfo(t *testing.T) {
	var buffer bytes.Buffer
	writer := bufio.NewWriter(&buffer)

	logger := New("test").WithOutput(NewLevelWriter(NewConsoleWriter(writer, false), Linfo))

	logger.Debug().Log("debug")
	writer.Flush()
	require.Equal(t, 0, buffer.Len(), "Buffer should be empty")
	buffer.Reset()

	logger.Info().Log("info")
	writer.Flush()
	require.NotEqual(t, 0, buffer.Len(), "Buffer should not be empty")
	buffer.Reset()

	logger.Warn().Log("warn")
	writer.Flush()
	require.NotEqual(t, 0, buffer.Len(), "Buffer should not be empty")
	buffer.Reset()

	logger.Error().Log("error")
	writer.Flush()
	require.NotEqual(t, 0, buffer.Len(), "Buffer should not be empty")
	buffer.Reset()
}

func TestLogWarn(t *testing.T) {
	var buffer bytes.Buffer
	writer := bufio.NewWriter(&buffer)

	logger := New("test").WithOutput(NewLevelWriter(NewConsoleWriter(writer, false), Lwarn))

	logger.Debug().Log("debug")
	writer.Flush()
	require.Equal(t, 0, buffer.Len(), "Buffer should be empty")
	buffer.Reset()

	logger.Info().Log("info")
	writer.Flush()
	require.Equal(t, 0, buffer.Len(), "Buffer should be empty")
	buffer.Reset()

	logger.Warn().Log("warn")
	writer.Flush()
	require.NotEqual(t, 0, buffer.Len(), "Buffer should not be empty")
	buffer.Reset()

	logger.Error().Log("error")
	writer.Flush()
	require.NotEqual(t, 0, buffer.Len(), "Buffer should not be empty")
	buffer.Reset()
}

func TestLogError(t *testing.T) {
	var buffer bytes.Buffer
	writer := bufio.NewWriter(&buffer)

	logger := New("test").WithOutput(NewLevelWriter(NewConsoleWriter(writer, false), Lerror))

	logger.Debug().Log("debug")
	writer.Flush()
	require.Equal(t, 0, buffer.Len(), "Buffer should be empty")
	buffer.Reset()

	logger.Info().Log("info")
	writer.Flush()
	require.Equal(t, 0, buffer.Len(), "Buffer should be empty")
	buffer.Reset()

	logger.Warn().Log("warn")
	writer.Flush()
	require.Equal(t, 0, buffer.Len(), "Buffer should be empty")
	buffer.Reset()

	logger.Error().Log("error")
	writer.Flush()
	require.NotEqual(t, 0, buffer.Len(), "Buffer should not be empty")
	buffer.Reset()
}
