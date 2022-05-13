package net

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIPLimiterNew(t *testing.T) {
	var err error

	_, err = NewIPLimiter([]string{}, []string{})
	assert.Nil(t, err)

	_, err = NewIPLimiter([]string{"::1/128", "127.0.0.1/32", ""}, []string{})
	assert.Nil(t, err)

	_, err = NewIPLimiter([]string{}, []string{"::1/128", "127.0.0.1/32", ""})
	assert.Nil(t, err)
}

func TestIPLimiterError(t *testing.T) {
	var err error

	_, err = NewIPLimiter([]string{}, []string{})
	assert.Nil(t, err)

	_, err = NewIPLimiter([]string{"::1"}, []string{})
	assert.NotNil(t, err, "Should not accept invalid IP")

	_, err = NewIPLimiter([]string{}, []string{"::1"})
	assert.NotNil(t, err, "Should not accept invalid IP")
}

func TestIPLimiterInvalidIPs(t *testing.T) {
	limiter, _ := NewIPLimiter([]string{}, []string{})

	assert.False(t, limiter.IsAllowed(""), "Invalid IP shouldn't be allowed")
}

func TestIPLimiterNoIPs(t *testing.T) {
	limiter, _ := NewIPLimiter([]string{}, []string{})

	assert.True(t, limiter.IsAllowed("127.0.0.1"), "IP should be allowed")
}

func TestIPLimiterAllowlist(t *testing.T) {
	limiter, _ := NewIPLimiter([]string{}, []string{"::1/128"})

	assert.False(t, limiter.IsAllowed("127.0.0.1"), "Unallowed IP shouldn't be allowed")
	assert.True(t, limiter.IsAllowed("::1"), "Allowed IP should be allowed")
}

func TestIPLimiterBlocklist(t *testing.T) {
	limiter, _ := NewIPLimiter([]string{"::1/128"}, []string{})

	assert.True(t, limiter.IsAllowed("127.0.0.1"), "Allowed IP should be allowed")
	assert.False(t, limiter.IsAllowed("::1"), "Unallowed IP shouldn't be allowed")
}
