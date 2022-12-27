// Package letsdebug provides an library, web API and CLI to provide diagnostic
// information for why a particular (FQDN, ACME Validation Method) pair *may* fail
// when attempting to issue an SSL Certificate from Let's Encrypt (https://letsencrypt.org).
//
// The usage cannot be generalized to other ACME providers, as the policies checked by this package
// are specific to Let's Encrypt, rather than being mandated by the ACME protocol.
//
// This package relies on libunbound.
package letsdebug

import (
	"fmt"
	"os"
	"reflect"
	"time"
)

// Options provide additional configuration to the various checkers
type Options struct {
	// HTTPRequestPath alters the /.well-known/acme-challenge/letsdebug-test to
	// /acme-challenge/acme-challenge/{{ HTTPRequestPath }}
	HTTPRequestPath string
	// HTTPExpectResponse causes the HTTP checker to require the remote server to
	// respond with specific content. If the content does not match, then the test
	// will fail with severity Error.
	HTTPExpectResponse string
}

// Check calls CheckWithOptions with default options
func Check(domain string, method ValidationMethod) (probs []Problem, retErr error) {
	return CheckWithOptions(domain, method, Options{})
}

// CheckWithOptions will run each checker against the domain and validation method provided.
// It is expected that this method may take a long time to execute, and may not be cancelled.
func CheckWithOptions(domain string, method ValidationMethod, opts Options) (probs []Problem, retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("panic: %v", r)
		}
	}()

	ctx := newScanContext()
	if opts.HTTPRequestPath != "" {
		ctx.httpRequestPath = opts.HTTPRequestPath
	}
	if opts.HTTPExpectResponse != "" {
		ctx.httpExpectResponse = opts.HTTPExpectResponse
	}

	domain = normalizeFqdn(domain)

	for _, checker := range checkers {
		t := reflect.TypeOf(checker)
		debug("[*] + %v\n", t)
		start := time.Now()
		checkerProbs, err := checker.Check(ctx, domain, method)
		debug("[*] - %v in %v\n", t, time.Since(start))
		if err == nil {
			if len(checkerProbs) > 0 {
				probs = append(probs, checkerProbs...)
			}
			// dont continue checking when a fatal error occurs
			if hasFatalProblem(probs) {
				break
			}
		} else if err != errNotApplicable {
			return nil, err
		}
	}
	return probs, nil
}

var isDebug *bool

func debug(format string, args ...interface{}) {
	if isDebug == nil {
		d := os.Getenv("LETSDEBUG_DEBUG") != ""
		isDebug = &d
	}
	if !(*isDebug) {
		return
	}
	fmt.Fprintf(os.Stderr, format, args...)
}
