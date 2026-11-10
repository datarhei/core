package packet

import (
	"encoding/binary"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/datarhei/gosrt/circular"
)

// 3.2.5.  NAK (Loss Report)

// CIFNAK represents a NAK message
type CIFNAK struct {
	LostPacketSequenceNumber []circular.Number
}

func (c CIFNAK) String() string {
	var b strings.Builder

	fmt.Fprintf(&b, "--- NAK ---\n")

	if len(c.LostPacketSequenceNumber)%2 != 0 {
		fmt.Fprintf(&b, "   invalid list of sequence numbers\n")
		return b.String()
	}

	for i := 0; i < len(c.LostPacketSequenceNumber); i += 2 {
		if c.LostPacketSequenceNumber[i].Equals(c.LostPacketSequenceNumber[i+1]) {
			fmt.Fprintf(&b, "   single: %#08x\n", c.LostPacketSequenceNumber[i].Val())
		} else {
			fmt.Fprintf(&b, "      row: %#08x to %#08x\n", c.LostPacketSequenceNumber[i].Val(), c.LostPacketSequenceNumber[i+1].Val())
		}
	}

	fmt.Fprintf(&b, "--- /NAK ---")

	return b.String()
}

func (c *CIFNAK) Unmarshal(data []byte) error {
	if len(data)%4 != 0 {
		return fmt.Errorf("data has wrong length to unmarshal")
	}

	// Appendix A

	c.LostPacketSequenceNumber = []circular.Number{}

	var sequenceNumber circular.Number
	isRange := false

	for i := 0; i < len(data); i += 4 {
		sequenceNumber = circular.New(binary.BigEndian.Uint32(data[i:])&MAX_SEQUENCENUMBER, MAX_SEQUENCENUMBER)

		if data[i]&0b10000000 == 0 {
			c.LostPacketSequenceNumber = append(c.LostPacketSequenceNumber, sequenceNumber)

			if !isRange {
				c.LostPacketSequenceNumber = append(c.LostPacketSequenceNumber, sequenceNumber)
			}

			isRange = false
		} else {
			c.LostPacketSequenceNumber = append(c.LostPacketSequenceNumber, sequenceNumber)
			isRange = true
		}
	}

	if len(c.LostPacketSequenceNumber)%2 != 0 {
		return fmt.Errorf("data too short to unmarshal")
	}

	sort.Slice(c.LostPacketSequenceNumber, func(i, j int) bool { return c.LostPacketSequenceNumber[i].Lt(c.LostPacketSequenceNumber[j]) })

	return nil
}

func (c *CIFNAK) Marshal(w io.Writer) error {
	if w == nil {
		return fmt.Errorf("invalid writer")
	}

	if len(c.LostPacketSequenceNumber)%2 != 0 {
		return fmt.Errorf("invalid length of lost packet sequence numbers")
	}

	// Appendix A

	var buffer [8]byte
	bytesWritten := 0

	for i := 0; i < len(c.LostPacketSequenceNumber); i += 2 {
		if c.LostPacketSequenceNumber[i] == c.LostPacketSequenceNumber[i+1] {
			binary.BigEndian.PutUint32(buffer[0:], c.LostPacketSequenceNumber[i].Val())
			w.Write(buffer[0:4])

			bytesWritten += 4
		} else {
			binary.BigEndian.PutUint32(buffer[0:], c.LostPacketSequenceNumber[i].Val()|0b10000000_00000000_00000000_00000000)
			binary.BigEndian.PutUint32(buffer[4:], c.LostPacketSequenceNumber[i+1].Val())
			w.Write(buffer[0:])

			bytesWritten += 8
		}

		if bytesWritten >= MAX_PAYLOAD_SIZE-4 {
			break
		}
	}

	return nil
}
