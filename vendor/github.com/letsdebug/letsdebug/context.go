package letsdebug

import (
	"fmt"
	"math/rand"
	"net"
	"sync"

	"github.com/miekg/dns"
)

type lookupResult struct {
	RRs   []dns.RR
	Error error
}

type scanContext struct {
	rrs      map[string]map[uint16]lookupResult
	rrsMutex sync.Mutex

	httpRequestPath    string
	httpExpectResponse string
}

func newScanContext() *scanContext {
	return &scanContext{
		rrs:             map[string]map[uint16]lookupResult{},
		httpRequestPath: "letsdebug-test",
	}
}

func (sc *scanContext) Lookup(name string, rrType uint16) ([]dns.RR, error) {
	sc.rrsMutex.Lock()
	rrMap, ok := sc.rrs[name]
	if !ok {
		rrMap = map[uint16]lookupResult{}
		sc.rrs[name] = rrMap
	}
	result, ok := rrMap[rrType]
	sc.rrsMutex.Unlock()

	if ok {
		return result.RRs, result.Error
	}

	resolved, err := lookup(name, rrType)

	sc.rrsMutex.Lock()
	rrMap[rrType] = lookupResult{
		RRs:   resolved,
		Error: err,
	}
	sc.rrsMutex.Unlock()

	return resolved, err
}

// Only slightly random - it will use AAAA over A if possible.
func (sc *scanContext) LookupRandomHTTPRecord(name string) (net.IP, error) {
	v6RRs, err := sc.Lookup(name, dns.TypeAAAA)
	if err != nil {
		return net.IP{}, err
	}
	if len(v6RRs) > 0 {
		if selected, ok := v6RRs[rand.Intn(len(v6RRs))].(*dns.AAAA); ok {
			return selected.AAAA, nil
		}
	}

	v4RRs, err := sc.Lookup(name, dns.TypeA)
	if err != nil {
		return net.IP{}, err
	}
	if len(v4RRs) > 0 {
		if selected, ok := v4RRs[rand.Intn(len(v4RRs))].(*dns.A); ok {
			return selected.A, nil
		}
	}

	return net.IP{}, fmt.Errorf("No AAAA or A records were found for %s", name)
}
