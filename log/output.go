package log

import (
	"io"
	"os"

	"github.com/mattn/go-isatty"
)

type consoleOutput struct {
	writer    io.Writer
	formatter Formatter
}

func NewConsoleOutput(w io.Writer, useColor bool) Writer {
	writer := &consoleOutput{
		writer: w,
	}

	color := useColor

	if color {
		if w, ok := w.(*os.File); ok {
			if !isatty.IsTerminal(w.Fd()) && !isatty.IsCygwinTerminal(w.Fd()) {
				color = false
			}
		} else {
			color = false
		}
	}

	writer.formatter = NewConsoleFormatter(color)

	return NewSyncWriter(writer)
}

func (w *consoleOutput) Write(e *Event) error {
	_, err := w.writer.Write(w.formatter.Bytes(e))

	return err
}

func (w *consoleOutput) Close() {}
