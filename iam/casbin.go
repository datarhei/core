package iam

import (
	"fmt"
	"strings"

	"github.com/gobwas/glob"
)

func resourceMatch(request, domain, policy string) bool {
	reqPrefix, reqResource := getPrefix(request)
	polPrefix, polResource := getPrefix(policy)

	if reqPrefix != polPrefix {
		return false
	}

	fmt.Printf("prefix: %s\n", reqPrefix)
	fmt.Printf("requested resource: %s\n", reqResource)
	fmt.Printf("requested domain: %s\n", domain)
	fmt.Printf("policy resource: %s\n", polResource)

	var match bool
	var err error

	if reqPrefix == "processid" {
		match, err = globMatch(polResource, reqResource)
		if err != nil {
			return false
		}
	} else if reqPrefix == "api" {
		match, err = globMatch(polResource, reqResource, rune('/'))
		if err != nil {
			return false
		}
	} else if reqPrefix == "fs" {
		match, err = globMatch(polResource, reqResource, rune('/'))
		if err != nil {
			return false
		}
	} else if reqPrefix == "rtmp" {
		match, err = globMatch(polResource, reqResource)
		if err != nil {
			return false
		}
	} else if reqPrefix == "srt" {
		match, err = globMatch(polResource, reqResource)
		if err != nil {
			return false
		}
	} else {
		match, err = globMatch(polResource, reqResource)
		if err != nil {
			return false
		}
	}

	fmt.Printf("match: %v\n", match)

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
	splits := strings.SplitN(s, ":", 2)

	if len(splits) == 0 {
		return "", ""
	}

	if len(splits) == 1 {
		return "", splits[0]
	}

	return splits[0], splits[1]
}

func globMatch(pattern, name string, separators ...rune) (bool, error) {
	g, err := glob.Compile(pattern, separators...)
	if err != nil {
		return false, err
	}

	return g.Match(name), nil
}
