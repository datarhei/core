package packet

import (
	"encoding/binary"
	"fmt"
	"io"
)

//  3.2.7. Shutdown

// CIFShutdown represents a shutdown message.
type CIFShutdown struct{}

func (c CIFShutdown) String() string {
	return "--- Shutdown ---"
}

func (c *CIFShutdown) Unmarshal(data []byte) error {
	if len(data) != 0 && len(data) != 4 {
		return fmt.Errorf("invalid length")
	}

	return nil
}

func (c *CIFShutdown) Marshal(w io.Writer) error {
	if w == nil {
		return fmt.Errorf("invalid writer")
	}

	var buffer [4]byte

	binary.BigEndian.PutUint32(buffer[0:], 0)

	_, err := w.Write(buffer[0:])

	return err
}
