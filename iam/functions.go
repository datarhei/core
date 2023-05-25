package iam

import (
	"strings"

	"github.com/gobwas/glob"
)

func resourceMatch(request, domain, policy string) bool {
	reqPrefix, reqResource := getPrefix(request)
	polPrefix, polResource := getPrefix(policy)

	if reqPrefix != polPrefix {
		return false
	}

	var match bool
	var err error

	if reqPrefix == "api" || reqPrefix == "fs" || reqPrefix == "rtmp" || reqPrefix == "srt" {
		match, err = globMatch(polResource, reqResource, rune('/'))
		if err != nil {
			return false
		}
	} else {
		match, err = globMatch(polResource, reqResource)
		if err != nil {
			return false
		}
	}

	return match
}

func resourceMatchFunc(args ...interface{}) (interface{}, error) {
	request := args[0].(string)
	domain := args[1].(string)
	policy := args[2].(string)

	return (bool)(resourceMatch(request, domain, policy)), nil
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

func globMatch(pattern, name string, separators ...rune) (bool, error) {
	g, err := glob.Compile(pattern, separators...)
	if err != nil {
		return false, err
	}

	return g.Match(name), nil
}
