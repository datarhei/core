package uuid

import (
	"uuid"

	"github.com/lithammer/shortuuid/v5"
)

const DefaultShortAlphabet = shortuuid.DefaultAlphabet

func New() string {
	return uuid.New().String()
}

func NewShort() string {
	return shortuuid.New()
}
