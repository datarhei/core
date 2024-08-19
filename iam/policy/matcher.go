package policy

import (
	"slices"
	"sync"

	"github.com/datarhei/core/v16/glob"
)

var globcache = map[string]glob.Glob{}
var globcacheMu = sync.RWMutex{}

func resourceMatch(requestType, requestResource string, policyTypes []string, policyResource string) bool {
	var match bool = false
	var err error = nil

	if !slices.Contains(policyTypes, requestType) {
		return false
	}

	key := requestType + policyResource

	if requestType == "api" || requestType == "fs" || requestType == "rtmp" || requestType == "srt" {
		globcacheMu.RLock()
		matcher, ok := globcache[key]
		globcacheMu.RUnlock()
		if !ok {
			matcher, err = glob.Compile(policyResource, rune('/'))
			if err != nil {
				return false
			}
			globcacheMu.Lock()
			globcache[key] = matcher
			globcacheMu.Unlock()
		}

		match = matcher.Match(requestResource)
	} else {
		globcacheMu.RLock()
		matcher, ok := globcache[key]
		globcacheMu.RUnlock()
		if !ok {
			matcher, err = glob.Compile(policyResource)
			if err != nil {
				return false
			}
			globcacheMu.Lock()
			globcache[key] = matcher
			globcacheMu.Unlock()
		}

		match = matcher.Match(requestResource)
	}

	return match
}

func actionMatch(requestAction string, policyActions []string, wildcard string) bool {
	if len(policyActions) == 0 {
		return false
	}

	if len(policyActions) == 1 && policyActions[0] == wildcard {
		return true
	}

	if slices.Contains(policyActions, requestAction) {
		return true
	}

	return false
}
