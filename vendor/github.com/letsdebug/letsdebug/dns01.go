package letsdebug

import (
	"crypto/rand"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/miekg/dns"
	"github.com/weppos/publicsuffix-go/publicsuffix"
)

// wildcardDNS01OnlyChecker ensures that a wildcard domain is only validated via dns-01.
type wildcardDNS01OnlyChecker struct{}

func (c wildcardDNS01OnlyChecker) Check(ctx *scanContext, domain string, method ValidationMethod) ([]Problem, error) {
	if !strings.HasPrefix(domain, "*.") {
		return nil, errNotApplicable
	}

	if method == DNS01 {
		return nil, errNotApplicable
	}

	return []Problem{wildcardHTTP01(domain, method)}, nil
}

func wildcardHTTP01(domain string, method ValidationMethod) Problem {
	return Problem{
		Name:        "MethodNotSuitable",
		Explanation: fmt.Sprintf("A wildcard domain like %s can only be issued using a dns-01 validation method.", domain),
		Detail:      fmt.Sprintf("Invalid method: %s", method),
		Severity:    SeverityFatal,
	}
}

// txtRecordChecker ensures there is no resolution errors with the _acme-challenge txt record
type txtRecordChecker struct{}

func (c txtRecordChecker) Check(ctx *scanContext, domain string, method ValidationMethod) ([]Problem, error) {
	if method != DNS01 {
		return nil, errNotApplicable
	}

	domain = strings.TrimPrefix(domain, "*.")

	if _, err := ctx.Lookup("_acme-challenge."+domain, dns.TypeTXT); err != nil {
		// report this problem as a fatal problem as that is the purpose of this checker
		return []Problem{txtRecordError(domain, err)}, nil
	}

	return nil, nil
}

func txtRecordError(domain string, err error) Problem {
	return Problem{
		Name: "TXTRecordError",
		Explanation: fmt.Sprintf(`An error occurred while attempting to lookup the TXT record on _acme-challenge.%s . `+
			`Any resolver errors that the Let's Encrypt CA encounters on this record will cause certificate issuance to fail.`, domain),
		Detail:   err.Error(),
		Severity: SeverityFatal,
	}
}

// txtDoubledLabelChecker ensures that a record for _acme-challenge.example.org.example.org
// wasn't accidentally created
type txtDoubledLabelChecker struct{}

func (c txtDoubledLabelChecker) Check(ctx *scanContext, domain string, method ValidationMethod) ([]Problem, error) {
	if method != DNS01 {
		return nil, errNotApplicable
	}

	registeredDomain, _ := publicsuffix.Domain(domain)

	variants := []string{
		fmt.Sprintf("_acme-challenge.%s.%s", domain, domain),           // _acme-challenge.www.example.org.www.example.org
		fmt.Sprintf("_acme-challenge.%s.%s", domain, registeredDomain), // _acme-challenge.www.example.org.example.org
	}

	var found []string
	distinctCombined := map[string]struct{}{}
	var randomCombined string

	var foundMu sync.Mutex

	var wg sync.WaitGroup
	wg.Add(len(variants) + 1)

	doQuery := func(q string) ([]string, string) {
		found := []string{}
		combined := []string{}
		rrs, _ := ctx.Lookup(q, dns.TypeTXT)
		for _, rr := range rrs {
			txt, ok := rr.(*dns.TXT)
			if !ok {
				continue
			}
			found = append(found, txt.String())
			combined = append(combined, txt.Txt...)
		}
		sort.Strings(combined)
		return found, strings.Join(combined, "\n")
	}

	// Check the double label variants
	for _, variant := range variants {
		go func(q string) {
			defer wg.Done()

			values, combined := doQuery(q)
			if len(values) == 0 {
				return
			}

			foundMu.Lock()
			defer foundMu.Unlock()

			found = append(found, values...)
			distinctCombined[combined] = struct{}{}
		}(variant)
	}

	// Check the response for a random subdomain, to detect the presence of a wildcard TXT record
	go func() {
		defer wg.Done()

		nonce := make([]byte, 4)
		_, _ = rand.Read(nonce)
		_, randomCombined = doQuery(fmt.Sprintf("_acme-challenge.%s.%s", fmt.Sprintf("rand-%x", nonce), domain))
	}()

	wg.Wait()

	// If a randomized subdomain has the exact same non-empty TXT response as any of the "double labels", then
	// we are probably dealing with a wildcard TXT record in the zone, and it is probably not a meaningful
	// misconfiguration. In this case, say nothing.
	if _, ok := distinctCombined[randomCombined]; ok && randomCombined != "" {
		return nil, nil
	}

	if len(found) > 0 {
		return []Problem{{
			Name: "TXTDoubleLabel",
			Explanation: "Some DNS records were found that indicate TXT records may have been incorrectly manually entered into " +
				`DNS editor interfaces. The correct way to enter these records is to either remove the domain from the label (so ` +
				`enter "_acme-challenge.www.example.org" as "_acme-challenge.www") or include a period (.) at the ` +
				`end of the label (enter "_acme-challenge.example.org.").`,
			Detail:   fmt.Sprintf("The following probably-erroneous TXT records were found:\n%s", strings.Join(found, "\n")),
			Severity: SeverityWarning,
		}}, nil
	}

	return nil, nil
}
