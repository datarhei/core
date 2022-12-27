// Package publicsuffix is a drop-in replacement for the golang.org/x/net/publicsuffix
// based on the weppos/publicsuffix package.
package publicsuffix

import (
	psl "github.com/weppos/publicsuffix-go/publicsuffix"
)

// PublicSuffix returns the public suffix of the domain
// using a copy of the publicsuffix.org database packaged into this library.
//
// Note. To maintain compatibility with the golang.org/x/net/publicsuffix
// this method doesn't return an error. However, in case of error,
// the returned value is empty.
func PublicSuffix(domain string) (publicSuffix string, icann bool) {
	//d, err := psl.Parse(domain)
	//if err != nil {
	//	return "", false
	//}
	//
	//return d.Rule.Value, !d.Rule.Private

	rule := psl.DefaultList.Find(domain, nil)
	publicSuffix = rule.Decompose(domain)[1]
	icann = !rule.Private

	// x/net/publicsuffix sets icann to false when the default rule "*" is used
	if rule.Value == "" && rule.Type == psl.WildcardType {
		icann = false
	}

	return
}

// EffectiveTLDPlusOne returns the effective top level domain plus one more label.
// For example, the eTLD+1 for "foo.bar.golang.org" is "golang.org".
func EffectiveTLDPlusOne(domain string) (string, error) {
	return psl.Domain(domain)
}
