package fec

import (
	"sync"

	"github.com/datarhei/gosrt/circular"
	"github.com/datarhei/gosrt/packet"
)

// Reconstructor is responsible for rebuilding lost packets using FEC control packets.
type Reconstructor struct {
	config Config

	mu sync.Mutex

	packets  map[uint32]packet.Packet
	controls map[uint32]*ControlPacket

	lastClean uint32
}

// NewReconstructor creates a new FEC Reconstructor.
func NewReconstructor(cfg Config) *Reconstructor {
	return &Reconstructor{
		config:   cfg,
		packets:  make(map[uint32]packet.Packet),
		controls: make(map[uint32]*ControlPacket),
	}
}

// AddPacket processes an incoming data packet or FEC control packet.
// It returns a slice of recovered packets (if any).
func (r *Reconstructor) AddPacket(p packet.Packet) []packet.Packet {
	r.mu.Lock()
	defer r.mu.Unlock()

	var recovered []packet.Packet

	if IsControlPacket(p) {
		ctrl, err := ParseControlPacket(p)
		if err != nil {
			return nil
		}

		r.controls[ctrl.SNBase] = ctrl

		if pkt := r.tryReconstruct(ctrl.SNBase); pkt != nil {
			recovered = append(recovered, pkt)
		}
	} else {
		// Data packet
		seq := p.Header().PacketSequenceNumber.Val()
		r.packets[seq] = p.Clone()

		// If we already have the control packet for this sequence, we might be able to reconstruct now
		// We'd have to find the SNBase. A simple way: check the past few sequence numbers.
		for base := seq - uint32(r.config.Cols); base <= seq; base++ {
			if _, ok := r.controls[base]; ok {
				if pkt := r.tryReconstruct(base); pkt != nil {
					recovered = append(recovered, pkt)
				}
				break
			}
		}

		// Periodically clean up old packets (keep a window of 100 packets)
		if len(r.packets) > 100 {
			for k := range r.packets {
				if circular.New(k, packet.MAX_SEQUENCENUMBER).Distance(circular.New(seq, packet.MAX_SEQUENCENUMBER)) > 100 {
					delete(r.packets, k)
				}
			}
			for k := range r.controls {
				if circular.New(k, packet.MAX_SEQUENCENUMBER).Distance(circular.New(seq, packet.MAX_SEQUENCENUMBER)) > 100 {
					delete(r.controls, k)
				}
			}
		}
	}

	return recovered
}

func (r *Reconstructor) tryReconstruct(snBase uint32) packet.Packet {
	ctrl, ok := r.controls[snBase]
	if !ok {
		return nil
	}

	missingCount := 0
	var missingSeq uint32

	for i := uint32(0); i < uint32(r.config.Cols); i++ {
		seq := (snBase + i) & packet.MAX_SEQUENCENUMBER
		if _, ok := r.packets[seq]; !ok {
			missingCount++
			missingSeq = seq
		}
	}

	if missingCount == 0 {
		// All packets received, clean up control
		delete(r.controls, snBase)
		return nil
	}

	if missingCount == 1 {
		// Exactly one missing! Reconstruct it.
		p := packet.NewPacket(nil)
		p.Header().IsControlPacket = false
		p.Header().PacketSequenceNumber = circular.New(missingSeq, packet.MAX_SEQUENCENUMBER)

		tsRecov := ctrl.TimestampRecov
		flagsRecov := ctrl.FlagsRecov
		lengthRecov := ctrl.LengthRecov

		// Determine max length we might need
		maxLen := len(ctrl.PayloadRecov)
		payloadRecov := make([]byte, maxLen)
		copy(payloadRecov, ctrl.PayloadRecov)

		for i := uint32(0); i < uint32(r.config.Cols); i++ {
			seq := (snBase + i) & packet.MAX_SEQUENCENUMBER
			if seq == missingSeq {
				continue
			}

			pkt := r.packets[seq]
			tsRecov ^= pkt.Header().Timestamp
			flagsRecov ^= byte(pkt.Header().KeyBaseEncryptionFlag.Val() & 0x03)
			lengthRecov ^= uint16(pkt.Len())

			data := pkt.Data()
			for j := 0; j < len(data); j++ {
				payloadRecov[j] ^= data[j]
			}
		}

		p.Header().Timestamp = tsRecov
		p.Header().KeyBaseEncryptionFlag = packet.PacketEncryption(flagsRecov & 0x03)
		p.Header().RetransmittedPacketFlag = true

		if int(lengthRecov) <= len(payloadRecov) {
			p.SetData(payloadRecov[:lengthRecov])
		} else {
			p.SetData(payloadRecov)
		}

		// Add the reconstructed packet to our buffer so it can be used for future blocks if needed (e.g. 2D layout)
		r.packets[missingSeq] = p.Clone()

		// Clean up control
		delete(r.controls, snBase)

		return p
	}

	return nil
}
