package net

import (
	"fmt"
	"net"
	"net/netip"
	"strings"
	"sync"
)

// The IPLimitValidator interface allows to check whether a certain IP is allowed.
type IPLimitValidator interface {
	// Tests whether the IP is allowed in respect to the underlying implementation
	IsAllowed(ip string) bool
}

type IPLimiter interface {
	// AddAllow adds a CIDR block to the allow list. If only an IP is provided
	// a CIDR will be generated.
	AddAllow(cidr string) error

	// RemoveAllow removes a CIDR block from the allow list. If only an IP is provided
	// a CIDR will be generated.
	RemoveAllow(cidr string) error

	// AddBlock adds a CIDR block to the block list. If only an IP is provided
	// a CIDR will be generated.
	AddBlock(cidr string) error

	// RemoveBlock removes a CIDR block from the block list. If only an IP is provided
	// a CIDR will be generated.
	RemoveBlock(cidr string) error

	IPLimitValidator
}

// IPLimit implements the IPLimiter interface by having an allow and block list
// of CIDR ranges.
type iplimit struct {
	// allowList is an array of allowed IP ranges
	allowlist map[string]*net.IPNet

	// blocklist is an array of blocked IP ranges
	blocklist map[string]*net.IPNet

	// lock is synchronizing the acces to the allow and block lists
	lock sync.RWMutex
}

// NewIPLimiter creates a new IPLimiter with the given IP ranges for the
// allowed and blocked IPs. Empty strings are ignored. Returns an error
// if an invalid IP range has been found.
func NewIPLimiter(blocklist, allowlist []string) (IPLimiter, error) {
	ipl := &iplimit{
		allowlist: make(map[string]*net.IPNet),
		blocklist: make(map[string]*net.IPNet),
	}

	for _, ipblock := range blocklist {
		err := ipl.AddBlock(ipblock)
		if err != nil {
			return nil, fmt.Errorf("block list: %w", err)
		}
	}

	for _, ipblock := range allowlist {
		err := ipl.AddAllow(ipblock)
		if err != nil {
			return nil, fmt.Errorf("allow list: %w", err)
		}
	}

	return ipl, nil
}

func (ipl *iplimit) validate(ipblock string) (*net.IPNet, error) {
	ipblock = strings.TrimSpace(ipblock)
	if len(ipblock) == 0 {
		return nil, fmt.Errorf("invalid IP block")
	}

	_, cidr, err := net.ParseCIDR(ipblock)
	if err != nil {
		addr, err := netip.ParseAddr(ipblock)
		if err != nil {
			return nil, fmt.Errorf("invalid IP block: %w", err)
		}

		if addr.Is4() {
			ipblock = addr.String() + "/32"
		} else {
			ipblock = addr.String() + "/128"
		}

		_, cidr, err = net.ParseCIDR(ipblock)
		if err != nil {
			return nil, fmt.Errorf("invalid IP block: %w", err)
		}
	}

	return cidr, nil
}

func (ipl *iplimit) AddAllow(ipblock string) error {
	cidr, err := ipl.validate(ipblock)
	if err != nil {
		return err
	}

	ipl.lock.Lock()
	defer ipl.lock.Unlock()

	ipl.allowlist[cidr.String()] = cidr

	return nil
}

func (ipl *iplimit) RemoveAllow(ipblock string) error {
	cidr, err := ipl.validate(ipblock)
	if err != nil {
		return err
	}

	ipl.lock.Lock()
	defer ipl.lock.Unlock()

	delete(ipl.allowlist, cidr.String())

	return nil
}

func (ipl *iplimit) AddBlock(ipblock string) error {
	cidr, err := ipl.validate(ipblock)
	if err != nil {
		return err
	}

	ipl.lock.Lock()
	defer ipl.lock.Unlock()

	ipl.blocklist[cidr.String()] = cidr

	return nil
}

func (ipl *iplimit) RemoveBlock(ipblock string) error {
	cidr, err := ipl.validate(ipblock)
	if err != nil {
		return err
	}

	ipl.lock.Lock()
	defer ipl.lock.Unlock()

	delete(ipl.blocklist, cidr.String())

	return nil
}

func (ipl *iplimit) IsAllowed(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	ipl.lock.RLock()
	defer ipl.lock.RUnlock()

	for _, r := range ipl.blocklist {
		if r.Contains(parsedIP) {
			return false
		}
	}

	if len(ipl.allowlist) == 0 {
		return true
	}

	for _, r := range ipl.allowlist {
		if r.Contains(parsedIP) {
			return true
		}
	}

	return false
}

func NewNullIPLimiter() IPLimiter {
	ipl, _ := NewIPLimiter(nil, nil)

	return ipl
}
