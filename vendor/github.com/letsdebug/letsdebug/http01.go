package letsdebug

import (
	"bytes"
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/miekg/dns"
)

var (
	likelyModemRouters              = []string{"micro_httpd", "cisco-IOS", "LANCOM", "Mini web server 1.0 ZTE corp 2005."}
	isLikelyNginxTestcookiePayloads = [][]byte{
		[]byte(`src="/aes.js"`),
		[]byte(`src="/aes.min.js"`),
		[]byte(`var a=toNumbers`)}
	isHTTP497Payloads = [][]byte{
		// httpd: https://github.com/apache/httpd/blob/e820d1ea4d3f1f5152574dbaa13979887a5c14b7/modules/ssl/ssl_engine_kernel.c#L322
		[]byte("You're speaking plain HTTP to an SSL-enabled server port"),
		// nginx: https://github.com/nginx/nginx/blob/15544440425008d5ad39a295b826665ad56fdc90/src/http/ngx_http_special_response.c#L274
		[]byte("400 The plain HTTP request was sent to HTTPS port"),
	}
)

// dnsAChecker checks if there are any issues in Unbound looking up the A and
// AAAA records for a domain (such as DNSSEC issues or dead nameservers)
type dnsAChecker struct{}

func (c dnsAChecker) Check(ctx *scanContext, domain string, method ValidationMethod) ([]Problem, error) {
	if method != HTTP01 {
		return nil, errNotApplicable
	}

	var probs []Problem
	var aRRs, aaaaRRs []dns.RR
	var aErr, aaaaErr error

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		aaaaRRs, aaaaErr = ctx.Lookup(domain, dns.TypeAAAA)
	}()

	go func() {
		defer wg.Done()
		aRRs, aErr = ctx.Lookup(domain, dns.TypeA)
	}()

	wg.Wait()

	if aErr != nil {
		probs = append(probs, dnsLookupFailed(domain, "A", aErr))
	}
	if aaaaErr != nil {
		probs = append(probs, dnsLookupFailed(domain, "AAAA", aaaaErr))
	}

	for _, rr := range aRRs {
		if aRR, ok := rr.(*dns.A); ok && isAddressReserved(aRR.A) {
			probs = append(probs, reservedAddress(domain, aRR.A.String()))
		}
	}
	for _, rr := range aaaaRRs {
		if aaaaRR, ok := rr.(*dns.AAAA); ok && isAddressReserved(aaaaRR.AAAA) {
			probs = append(probs, reservedAddress(domain, aaaaRR.AAAA.String()))
		}
	}

	var sb []string
	for _, rr := range append(aRRs, aaaaRRs...) {
		sb = append(sb, rr.String())
	}

	if len(sb) > 0 {
		probs = append(probs, debugProblem("HTTPRecords", "A and AAAA records found for this domain", strings.Join(sb, "\n")))
	}

	if len(sb) == 0 {
		probs = append(probs, noRecords(domain, "No A or AAAA records found."))
	}

	return probs, nil
}

// httpAccessibilityChecker checks whether an HTTP ACME validation request
// would lead to any issues such as:
// - Bad redirects
// - IPs not listening on port 80
type httpAccessibilityChecker struct{}

func (c httpAccessibilityChecker) Check(ctx *scanContext, domain string, method ValidationMethod) ([]Problem, error) {
	if method != HTTP01 {
		return nil, errNotApplicable
	}

	var probs []Problem

	var ips []net.IP

	rrs, _ := ctx.Lookup(domain, dns.TypeAAAA)
	for _, rr := range rrs {
		aaaa, ok := rr.(*dns.AAAA)
		if !ok {
			continue
		}
		ips = append(ips, aaaa.AAAA)
	}
	rrs, _ = ctx.Lookup(domain, dns.TypeA)
	for _, rr := range rrs {
		a, ok := rr.(*dns.A)
		if !ok {
			continue
		}
		ips = append(ips, a.A)
	}

	if len(ips) == 0 {
		return probs, nil
	}

	// Track whether responses differ between any of the A/AAAA addresses
	// for the domain
	allCheckResults := []httpCheckResult{}

	var debug []string

	for _, ip := range ips {
		res, prob := checkHTTP(ctx, domain, ip)
		allCheckResults = append(allCheckResults, res)
		if !prob.IsZero() {
			probs = append(probs, prob)
		}
		debug = append(debug, fmt.Sprintf("Request to: %s/%s, Result: %s, Issue: %s\nTrace:\n%s\n",
			domain, ip.String(), res.String(), prob.Name, strings.Join(res.DialStack, "\n")))
	}

	// Filter out the servers that didn't respond at all
	var nonZeroResults []httpCheckResult
	for _, v := range allCheckResults {
		if v.IsZero() {
			continue
		}
		nonZeroResults = append(nonZeroResults, v)
	}
	if len(nonZeroResults) > 1 {
		firstResult := nonZeroResults[0]
		for _, otherResult := range nonZeroResults[1:] {
			if firstResult.StatusCode != otherResult.StatusCode ||
				firstResult.ServerHeader != otherResult.ServerHeader ||
				firstResult.NumRedirects != otherResult.NumRedirects ||
				firstResult.InitialStatusCode != otherResult.InitialStatusCode {
				probs = append(probs, multipleIPAddressDiscrepancy(domain, firstResult, otherResult))
			}
		}
	}

	probs = append(probs, debugProblem("HTTPCheck", "Requests made to the domain", strings.Join(debug, "\n")))

	if res := isLikelyModemRouter(allCheckResults); !res.IsZero() {
		probs = append(probs, Problem{
			Name: "PortForwarding",
			Explanation: "A request to your domain revealed that the web server that responded may be " +
				"the administrative interface of a modem or router. This can indicate an issue with the port forwarding " +
				"setup on that modem or router. You may need to reconfigure the device to properly forward traffic to your " +
				"intended webserver.",
			Detail: fmt.Sprintf(`The web server that responded identified itself as "%s", `+
				"which is a known webserver commonly used by modems/routers.", res.ServerHeader),
			Severity: SeverityWarning,
		})
	}

	if res := isLikelyNginxTestcookie(allCheckResults); !res.IsZero() {
		probs = append(probs, Problem{
			Name: "BlockedByNginxTestCookie",
			Explanation: "The validation request to this domain was blocked by a deployment of the nginx " +
				"testcookie module (https://github.com/kyprizel/testcookie-nginx-module). This module is designed to " +
				"block robots, and causes the Let's Encrypt validation process to fail. The server administrator can " +
				"solve this issue by disabling the module (`testcookie off;`) for requests under the path of `/.well-known" +
				"/acme-challenge/`.",
			Detail:   fmt.Sprintf("The server at %s produced this result.", res.IP.String()),
			Severity: SeverityError,
		})
	}

	if res := isHTTP497(allCheckResults); !res.IsZero() {
		probs = append(probs, Problem{
			Name: "HttpOnHttpsPort",
			Explanation: "A validation request to this domain resulted in an HTTP request being made to a port that expects " +
				"to receive HTTPS requests. This could be the result of an incorrect redirect (such as to http://example.com:443/) " +
				"or it could be the result of a webserver misconfiguration, such as trying to enable SSL on a port 80 virtualhost.",
			Detail:   strings.Join(res.DialStack, "\n"),
			Severity: SeverityError,
		})
	}

	return probs, nil
}

func noRecords(name, rrSummary string) Problem {
	return Problem{
		Name: "NoRecords",
		Explanation: fmt.Sprintf(`No valid A or AAAA records could be ultimately resolved for %s. `+
			`This means that Let's Encrypt would not be able to to connect to your domain to perform HTTP validation, since `+
			`it would not know where to connect to.`, name),
		Detail:   rrSummary,
		Severity: SeverityFatal,
	}
}

func reservedAddress(name, address string) Problem {
	return Problem{
		Name: "ReservedAddress",
		Explanation: fmt.Sprintf(`A private, inaccessible, IANA/IETF-reserved IP address was found for %s. Let's Encrypt will always fail HTTP validation `+
			`for any domain that is pointing to an address that is not routable on the internet. You should either remove this address `+
			`and replace it with a public one or use the DNS validation method instead.`, name),
		Detail:   address,
		Severity: SeverityFatal,
	}
}

func multipleIPAddressDiscrepancy(domain string, result1, result2 httpCheckResult) Problem {
	return Problem{
		Name: "MultipleIPAddressDiscrepancy",
		Explanation: fmt.Sprintf(`%s has multiple IP addresses in its DNS records. While they appear to be accessible on the network, `+
			`we have detected that they produce differing results when sent an ACME HTTP validation request. This may indicate that `+
			`some of the IP addresses may unintentionally point to different servers, which would cause validation to fail.`,
			domain),
		Detail:   fmt.Sprintf("%s vs %s", result1.String(), result2.String()),
		Severity: SeverityWarning,
	}
}

func isLikelyModemRouter(results []httpCheckResult) httpCheckResult {
	for _, res := range results {
		for _, toMatch := range likelyModemRouters {
			if res.ServerHeader == toMatch {
				return res
			}
		}
	}
	return httpCheckResult{}
}

func isLikelyNginxTestcookie(results []httpCheckResult) httpCheckResult {
	for _, res := range results {
		for _, needle := range isLikelyNginxTestcookiePayloads {
			if bytes.Contains(res.Content, needle) {
				return res
			}
		}
	}
	return httpCheckResult{}
}

func isHTTP497(results []httpCheckResult) httpCheckResult {
	for _, res := range results {
		for _, needle := range isHTTP497Payloads {
			if bytes.Contains(res.Content, needle) {
				return res
			}
		}
	}
	return httpCheckResult{}
}
