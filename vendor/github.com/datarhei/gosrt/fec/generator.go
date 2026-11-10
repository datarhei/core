package fec

import (
	"sync"

	"github.com/datarhei/gosrt/packet"
)

// Generator is responsible for generating FEC control packets.
type Generator struct {
	config Config

	mu     sync.Mutex
	buffer []packet.Packet
}

// NewGenerator creates a new FEC Generator.
func NewGenerator(cfg Config) *Generator {
	return &Generator{
		config: cfg,
		buffer: make([]packet.Packet, 0, cfg.Cols),
	}
}

// AddPacket processes an outgoing data packet and potentially generates an FEC control packet.
// Returns a slice of FEC control packets that are ready to be sent.
func (g *Generator) AddPacket(p packet.Packet) []packet.Packet {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Simplest 1D row-based FEC generation for illustration (to be expanded)
	// We only XOR the payload bytes here.
	clone := p.Clone()
	g.buffer = append(g.buffer, clone)

	if len(g.buffer) >= g.config.Cols {
		// Generate one FEC control packet for the row
		ctrl := g.generateRowFEC()

		// Reset buffer for the next row
		g.buffer = g.buffer[:0]

		// Create the gosrt packet
		fecPkt := WriteControlPacket(p.Header().DestinationSocketId, ctrl)
		return []packet.Packet{fecPkt}
	}

	return nil
}

func (g *Generator) generateRowFEC() *ControlPacket {
	if len(g.buffer) == 0 {
		return nil
	}

	ctrl := &ControlPacket{
		SNBase:         g.buffer[0].Header().PacketSequenceNumber.Val(),
		GroupIndex:     -1, // -1 means row
		TimestampRecov: 0,
		FlagsRecov:     0,
		LengthRecov:    0,
	}

	// Determine max payload size
	maxLen := 0
	for _, p := range g.buffer {
		if int(p.Len()) > maxLen {
			maxLen = int(p.Len())
		}
	}

	ctrl.PayloadRecov = make([]byte, maxLen)

	for _, p := range g.buffer {
		ctrl.TimestampRecov ^= p.Header().Timestamp
		// Flags: we'd XOR KK flags here
		flags := byte(p.Header().KeyBaseEncryptionFlag.Val() & 0x03)
		ctrl.FlagsRecov ^= flags

		length := uint16(p.Len())
		ctrl.LengthRecov ^= length

		data := p.Data()
		for i := 0; i < len(data); i++ {
			ctrl.PayloadRecov[i] ^= data[i]
		}
	}

	return ctrl
}
