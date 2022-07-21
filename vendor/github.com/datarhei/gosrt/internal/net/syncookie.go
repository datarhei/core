package net

import (
	"crypto/md5"
	"encoding/binary"
	"math/rand"
	"strconv"
	"time"
)

type SYNCookie struct {
	secret1 string
	secret2 string
	daddr   string
	counter func() int64
}

func defaultCounter() int64 {
	return time.Now().Unix() >> 6
}

func NewSYNCookie(daddr string, seed int64, counter func() int64) SYNCookie {
	s := SYNCookie{
		daddr:   daddr,
		counter: counter,
	}

	if s.counter == nil {
		s.counter = defaultCounter
	}

	// https://www.calhoun.io/creating-random-strings-in-go/
	charset := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	seededRand := rand.New(rand.NewSource(seed))

	stringWithCharset := func(length int, charset string) string {
		b := make([]byte, length)
		for i := range b {
			b[i] = charset[seededRand.Intn(len(charset))]
		}
		return string(b)
	}

	s.secret1 = stringWithCharset(32, charset)
	s.secret2 = stringWithCharset(32, charset)

	return s
}

func (s *SYNCookie) Get(saddr string) uint32 {
	return s.calculate(s.counter(), saddr)
}

func (s *SYNCookie) Verify(cookie uint32, saddr string) bool {
	counter := s.counter()

	if s.calculate(counter, saddr) == cookie {
		return true
	}

	if s.calculate(counter-1, saddr) == cookie {
		return true
	}

	return false
}

func (s *SYNCookie) calculate(counter int64, saddr string) uint32 {
	data := s.secret1 + s.daddr + saddr + s.secret2 + strconv.FormatInt(counter, 10)

	md5sum := md5.Sum([]byte(data))

	return binary.BigEndian.Uint32(md5sum[0:])
}
