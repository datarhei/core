package net

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAnonymizeIPString(t *testing.T) {
	ipv4 := "192.168.1.42"
	ipv6 := "bbd1:e95a:adbb:b29a:e38b:577f:6f9a:1fa7"

	anonymizedIPv4, err := AnonymizeIPString(ipv4)
	assert.Nil(t, err)
	assert.Equal(t, "192.168.1.0", anonymizedIPv4)

	anonymizedIPv6, err := AnonymizeIPString(ipv6)
	assert.Nil(t, err)
	assert.Equal(t, "bbd1:e95a:adbb:b29a::", anonymizedIPv6)
}
