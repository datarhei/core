package net

// from github.com/simplesurance/go-ip-anonymizer

import (
	"fmt"
	"net"
)

var (
	defaultIPv4Mask = net.IPv4Mask(255, 255, 255, 0)                                                     // /24
	defaultIPv6Mask = net.IPMask{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0, 0, 0, 0, 0, 0, 0, 0} // /64
)

func AnonymizeIPString(ipAddress string) (string, error) {
	switch ipVersion(ipAddress) {
	case 4:
		ip4 := net.ParseIP(ipAddress)
		if ip4 == nil {
			return ipAddress, fmt.Errorf("invalid IPv4 address")
		}

		return AnonymizeIPv4(ip4).String(), nil

	case 6:
		ipv6 := net.ParseIP(ipAddress)
		if ipv6 == nil {
			return ipAddress, fmt.Errorf("invalid IPv6 address")
		}

		return AnonymizeIPv6(ipv6).String(), nil

	default:
		return ipAddress, fmt.Errorf("invalid IP address")
	}
}

// IPv4 anonymizes an IPv4 address by zeroing it's host part.
func AnonymizeIPv4(ip net.IP) net.IP {
	return ip.Mask(defaultIPv4Mask)
}

// IPv6 anonymizes an IPv4 address by zeroing it's host part.
func AnonymizeIPv6(ip net.IP) net.IP {
	return ip.Mask(defaultIPv6Mask)
}

func ipVersion(ipAddress string) int {
	// copied from net.ParseIP()
	for i := 0; i < len(ipAddress); i++ {
		switch ipAddress[i] {
		case '.':
			return 4
		case ':':
			return 6
		}
	}

	return 0
}
