package rand

// https://www.calhoun.io/creating-random-strings-in-go/

import (
	"math/rand"
	"time"
)

const (
	CharsetLetters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	CharsetNumbers = "1234567890"
	CharsetSymbols = "#@+*%&/<>[]()=?!$.,:;-_"

	CharsetAll = CharsetLetters + CharsetNumbers + CharsetSymbols
)

var seededRand *rand.Rand = rand.New(rand.NewSource(time.Now().UnixNano()))

func StringWithCharset(length int, charset string) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}

	return string(b)
}

func StringLetters(length int) string {
	return StringWithCharset(length, CharsetLetters)
}

func StringNumbers(length int) string {
	return StringWithCharset(length, CharsetNumbers)
}

func StringAlphanumeric(length int) string {
	return StringWithCharset(length, CharsetLetters+CharsetNumbers)
}

func String(length int) string {
	return StringWithCharset(length, CharsetAll)
}
