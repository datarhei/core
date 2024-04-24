package log

import (
	"io"
	"strings"

	"github.com/datarhei/core/v16/encoding/json"
)

type logwrapper struct {
	writer io.Writer
}

type logentry struct {
	Message string `json:"message"`
}

func NewWrapper(writer io.Writer) *logwrapper {
	return &logwrapper{
		writer: writer,
	}
}

func (b *logwrapper) Write(p []byte) (int, error) {
	log := logentry{}
	if err := json.Unmarshal(p, &log); err == nil {
		if len(log.Message) != 0 {
			lines := strings.Split(log.Message, "\n")

			for _, line := range lines {
				b.writer.Write([]byte(line))
			}

			return len(p), nil
		}
	}

	return b.writer.Write(p)
}
