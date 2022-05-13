package parse

type statsData struct {
	frame  uint64
	packet uint64
	size   uint64 // kbytes
	dup    uint64
	drop   uint64
}

type stats struct {
	last statsData
	diff statsData
}

func (s *stats) updateFromProgress(p *ffmpegProgress) {
	s.diff.frame = p.Frame - s.last.frame
	s.diff.packet = p.Packet - s.last.packet
	s.diff.size = p.Size - s.last.size
	s.diff.drop = p.Drop - s.last.drop
	s.diff.dup = p.Dup - s.last.dup

	s.last.frame = p.Frame
	s.last.packet = p.Packet
	s.last.size = p.Size
	s.last.dup = p.Dup
	s.last.drop = p.Drop
}

func (s *stats) updateFromProgressIO(p *ffmpegProgressIO) {
	s.diff.frame = p.Frame - s.last.frame
	s.diff.packet = p.Packet - s.last.packet
	s.diff.size = p.Size - s.last.size

	s.last.frame = p.Frame
	s.last.packet = p.Packet
	s.last.size = p.Size
}
