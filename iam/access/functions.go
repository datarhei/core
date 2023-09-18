package access

import (
	"strings"

	"github.com/datarhei/core/v16/glob"
)

func resourceMatch(request, policy string) bool {
	reqPrefix, reqResource := getPrefix(request)
	polPrefix, polResource := getPrefix(policy)

	var match bool = false
	var err error = nil

	reqType := strings.ToLower(reqPrefix)
	polTypes := strings.Split(strings.ToLower(polPrefix), "|")

	for _, polType := range polTypes {
		if reqType != polType {
			continue
		}

		match = true
		break
	}

	if !match {
		return false
	}

	match = false

	if reqType == "api" || reqType == "fs" || reqType == "rtmp" || reqType == "srt" {
		match, err = glob.Match(polResource, reqResource, rune('/'))
		if err != nil {
			return false
		}
	} else {
		match, err = glob.Match(polResource, reqResource)
		if err != nil {
			return false
		}
	}

	return match
}

func resourceMatchFunc(args ...interface{}) (interface{}, error) {
	request := args[0].(string)
	policy := args[1].(string)

	return (bool)(resourceMatch(request, policy)), nil
}

func actionMatch(request string, policy string) bool {
	request = strings.ToUpper(request)
	actions := strings.Split(strings.ToUpper(policy), "|")
	if len(actions) == 0 {
		return false
	}

	if len(actions) == 1 && actions[0] == "ANY" {
		return true
	}

	for _, a := range actions {
		if request == a {
			return true
		}
	}

	return false
}

func actionMatchFunc(args ...interface{}) (interface{}, error) {
	request := args[0].(string)
	policy := args[1].(string)

	return (bool)(actionMatch(request, policy)), nil
}

func getPrefix(s string) (string, string) {
	prefix, resource, found := strings.Cut(s, ":")
	if !found {
		return "", s
	}

	return prefix, resource
}
