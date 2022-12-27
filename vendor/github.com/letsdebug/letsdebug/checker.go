package letsdebug

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"reflect"
	"time"
)

// ValidationMethod represents an ACME validation method
type ValidationMethod string

const (
	HTTP01    ValidationMethod = "http-01"     // HTTP01 represents the ACME http-01 validation method.
	DNS01     ValidationMethod = "dns-01"      // DNS01 represents the ACME dns-01 validation method.
	TLSALPN01 ValidationMethod = "tls-alpn-01" // TLSALPN01 represents the ACME tls-alpn-01 validation method.
)

var (
	validMethods     = map[ValidationMethod]bool{HTTP01: true, DNS01: true, TLSALPN01: true}
	errNotApplicable = errors.New("Checker not applicable for this domain and method")
	checkers         []checker
)

func init() {
	// Since the OFAC SDN checker polls, we need to initialize it
	ofac := &ofacSanctionChecker{}
	ofac.setup()

	// We want to launch the slowest checkers as early as possible,
	// unless they have a dependency on an earlier checker
	checkers = []checker{
		asyncCheckerBlock{
			validMethodChecker{},
			validDomainChecker{},
			wildcardDNS01OnlyChecker{},
			statusioChecker{},
			ofac,
		},

		asyncCheckerBlock{
			caaChecker{},             // depends on valid*Checker
			&rateLimitChecker{},      // depends on valid*Checker
			dnsAChecker{},            // depends on valid*Checker
			txtRecordChecker{},       // depends on valid*Checker
			txtDoubledLabelChecker{}, // depends on valid*Checker
		},

		asyncCheckerBlock{
			httpAccessibilityChecker{}, // depends on dnsAChecker
			cloudflareChecker{},        // depends on dnsAChecker to some extent
			&acmeStagingChecker{},      // Gets the final word
		},
	}
}

type checker interface {
	Check(ctx *scanContext, domain string, method ValidationMethod) ([]Problem, error)
}

// asyncCheckerBlock represents a checker which is composed of other checkers that can be run simultaneously.
type asyncCheckerBlock []checker

type asyncResult struct {
	Problems []Problem
	Error    error
}

func (c asyncCheckerBlock) Check(ctx *scanContext, domain string, method ValidationMethod) ([]Problem, error) {
	resultCh := make(chan asyncResult, len(c))

	id := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%d", time.Now().UnixNano()))))[:4]
	debug("[%s] Launching async\n", id)

	for _, task := range c {
		go func(task checker, ctx *scanContext, domain string, method ValidationMethod) {
			defer func() {
				if r := recover(); r != nil {
					resultCh <- asyncResult{nil, fmt.Errorf("Check %T paniced: %v", task, r)}
				}
			}()
			t := reflect.TypeOf(task)
			debug("[%s] async: + %v\n", id, t)
			start := time.Now()
			probs, err := task.Check(ctx, domain, method)
			debug("[%s] async: - %v in %v\n", id, t, time.Since(start))
			resultCh <- asyncResult{probs, err}
		}(task, ctx, domain, method)
	}

	var probs []Problem

	for i := 0; i < len(c); i++ {
		result := <-resultCh
		if result.Error != nil && result.Error != errNotApplicable {
			debug("[%s] Exiting async via error\n", id)
			return nil, result.Error
		}
		if len(result.Problems) > 0 {
			probs = append(probs, result.Problems...)
		}
	}

	debug("[%s] Exiting async gracefully\n", id)
	return probs, nil
}
