package mem

// Based on github.com/valyala/bytebufferpool

import (
	"bytes"
	"errors"
	"io"
)

type Buffer struct {
	data []byte
}

// Len returns the length of the buffer.
func (b *Buffer) Len() int {
	return len(b.data)
}

// Bytes returns the buffer, but keeps ownership.
func (b *Buffer) Bytes() []byte {
	return b.data
}

// WriteTo writes the bytes to the writer.
func (b *Buffer) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(b.data)
	return int64(n), err
}

// Reset empties the buffer and keeps it's capacity.
func (b *Buffer) Reset() {
	b.data = b.data[:0]
}

// Write appends to the buffer.
func (b *Buffer) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	return len(p), nil
}

// ReadFrom reads from the reader and appends to the buffer.
func (b *Buffer) ReadFrom(r io.Reader) (int64, error) {
	/*
		chunkData := [128 * 1024]byte{}
		chunk := chunkData[0:]

		size := int64(0)

		for {
			n, err := r.Read(chunk)
			if n != 0 {
				b.data = append(b.data, chunk[:n]...)
				size += int64(n)
			}

			if err != nil {
				if errors.Is(err, io.EOF) {
					return size, nil
				}

				return size, err
			}

			if n == 0 {
				break
			}
		}

		return size, nil
	*/
	p := b.data
	nStart := int64(len(p))
	nMax := int64(cap(p))
	n := nStart
	if nMax == 0 {
		nMax = 64
		p = make([]byte, nMax)
	} else {
		p = p[:nMax]
	}
	for {
		if n == nMax {
			nMax *= 2
			bNew := make([]byte, nMax)
			copy(bNew, p)
			p = bNew
		}
		nn, err := r.Read(p[n:])
		n += int64(nn)
		if err != nil {
			b.data = p[:n]
			n -= nStart
			if errors.Is(err, io.EOF) {
				return n, nil
			}
			return n, err
		}
	}
	/*
		if br, ok := r.(*bytes.Reader); ok {
			if cap(b.data) < br.Len() {
				data := make([]byte, br.Len())
				copy(data, b.data)
				b.data = data
			}
		}

		chunkData := [128 * 1024]byte{}
		chunk := chunkData[0:]

		size := int64(0)

		for {
			n, err := r.Read(chunk)
			if n != 0 {
				if cap(b.data) < len(b.data)+n {
					data := make([]byte, cap(b.data)+1024*1024)
					copy(data, b.data)
					b.data = data
				}
				b.data = append(b.data, chunk[:n]...)
				size += int64(n)
			}

			if err != nil {
				if errors.Is(err, io.EOF) {
					return size, nil
				}

				return size, err
			}

			if n == 0 {
				break
			}
		}

		return size, nil
	*/
}

// WriteByte appends a byte to the buffer.
func (b *Buffer) WriteByte(c byte) error {
	b.data = append(b.data, c)
	return nil
}

// WriteString appends a string to the buffer.
func (b *Buffer) WriteString(s string) (n int, err error) {
	b.data = append(b.data, s...)
	return len(s), nil
}

// Reader returns a bytes.Reader based on the data in the buffer.
func (b *Buffer) Reader() *bytes.Reader {
	return bytes.NewReader(b.data)
}

// String returns the data in the buffer a string.
func (b *Buffer) String() string {
	return string(b.data)
}
