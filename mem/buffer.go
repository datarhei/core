package mem

import (
	"bytes"
	"errors"
	"io"
)

type Buffer struct {
	data bytes.Buffer
}

// Len returns the length of the buffer.
func (b *Buffer) Len() int {
	return b.data.Len()
}

// Bytes returns the buffer, but keeps ownership.
func (b *Buffer) Bytes() []byte {
	return b.data.Bytes()
}

// WriteTo writes the bytes to the writer.
func (b *Buffer) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(b.data.Bytes())
	return int64(n), err
}

// Reset empties the buffer and keeps it's capacity.
func (b *Buffer) Reset() {
	b.data.Reset()
}

// Write appends to the buffer.
func (b *Buffer) Write(p []byte) (int, error) {
	return b.data.Write(p)
}

// ReadFrom reads from the reader and appends to the buffer.
func (b *Buffer) ReadFrom(r io.Reader) (int64, error) {
	if br, ok := r.(*bytes.Reader); ok {
		b.data.Grow(br.Len())
	}

	chunkData := [128 * 1024]byte{}
	chunk := chunkData[0:]

	size := int64(0)

	for {
		n, err := r.Read(chunk)
		if n != 0 {
			b.data.Write(chunk[:n])
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
}

// WriteByte appends a byte to the buffer.
func (b *Buffer) WriteByte(c byte) error {
	return b.data.WriteByte(c)
}

// WriteString appends a string to the buffer.
func (b *Buffer) WriteString(s string) (n int, err error) {
	return b.data.WriteString(s)
}

// Reader returns a bytes.Reader based on the data in the buffer.
func (b *Buffer) Reader() *bytes.Reader {
	return bytes.NewReader(b.Bytes())
}

// String returns the data in the buffer a string.
func (b *Buffer) String() string {
	return b.data.String()
}
