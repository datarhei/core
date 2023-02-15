package url

import (
	"net"
	"net/url"
	"regexp"
)

var reScheme = regexp.MustCompile(`(?i)^([a-z][a-z0-9.+-:]*)://`)

// Validate checks whether the given address is a valid URL
func Validate(address string) error {
	_, err := Parse(address)

	return err
}

// Parse parses an URL into its components. Returns a net/url.URL or
// an error if the URL couldn't be parsed.
func Parse(address string) (*url.URL, error) {
	u, err := url.Parse(address)

	return u, err
}

// HasScheme returns whether the address has an URL scheme prefix
func HasScheme(address string) bool {
	if ok := reScheme.MatchString(address); ok {
		return true
	}

	return false
}

// Lookup returns the first resolved IP address of the host in the address.
// If the address doesn't have an URL scheme prefix, an empty string and no
// error is returned. If the address is an URL and the lookup fails, an
// error is returned.
func Lookup(address string) (string, error) {
	if !HasScheme(address) {
		return "", nil
	}

	u, err := Parse(address)
	if err != nil {
		return "", err
	}

	if len(u.Host) == 0 {
		return "", nil
	}

	host, _, err := net.SplitHostPort(u.Host)
	if err != nil {
		host = u.Host
	}

	addrs, err := net.LookupHost(host)
	if err != nil {
		return "", err
	}

	return addrs[0], nil
}
