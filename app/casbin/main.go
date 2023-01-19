package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/gobwas/glob"
)

func main() {
	var subject string
	var domain string
	var object string
	var action string

	flag.StringVar(&subject, "subject", "$anon", "subject of this request")
	flag.StringVar(&domain, "domain", "$none", "domain of this request")
	flag.StringVar(&object, "object", "", "object of this request")
	flag.StringVar(&action, "action", "", "action of this request")

	flag.Parse()

	m := model.NewModel()
	m.AddDef("r", "r", "sub, dom, obj, act")
	m.AddDef("p", "p", "sub, dom, obj, act")
	m.AddDef("g", "g", "_, _, _")
	m.AddDef("e", "e", "some(where (p.eft == allow))")
	m.AddDef("m", "m", `g(r.sub, p.sub, r.dom) && r.dom == p.dom && ResourceMatch(r.obj, r.dom, p.obj) && ActionMatch(r.act, p.act) || r.sub == "$superuser"`)

	a := NewAdapter("./policy.json")

	e, err := casbin.NewEnforcer(m, a)
	if err != nil {
		fmt.Printf("error: %s\n", err)
		os.Exit(1)
	}

	e.AddFunction("ResourceMatch", ResourceMatchFunc)
	e.AddFunction("ActionMatch", ActionMatchFunc)

	/*
		if err := addGroup(e, "foobar"); err != nil {
			fmt.Printf("error: %s\n", err)
			os.Exit(1)
		}

		if err := addGroupUser(e, "foobar", "franz", "admin"); err != nil {
			fmt.Printf("error: %s\n", err)
			os.Exit(1)
		}

		if err := addGroupUser(e, "foobar", "$anon", "anonymous"); err != nil {
			fmt.Printf("error: %s\n", err)
			os.Exit(1)
		}

		e.RemovePolicy("bob", "igelcamp", "processid:*", "COMMAND")
		e.AddPolicy("bob", "igelcamp", "processid:bob-*", "COMMAND")
	*/
	ok, reason, err := e.EnforceEx(subject, domain, object, action)
	if err != nil {
		fmt.Printf("error: %s\n", err)
		os.Exit(1)
	}

	if ok {
		fmt.Printf("OK: %v\n", reason)
	} else {
		fmt.Printf("not OK\n")
	}
}

func ResourceMatch(request, domain, policy string) bool {
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
		match, err = Match(polResource, reqResource)
		if err != nil {
			return false
		}
	} else if reqPrefix == "api" {
		match, err = Match(polResource, reqResource, rune('/'))
		if err != nil {
			return false
		}
	} else if reqPrefix == "fs" {
		match, err = Match(polResource, reqResource, rune('/'))
		if err != nil {
			return false
		}
	} else if reqPrefix == "rtmp" {
		match, err = Match(polResource, reqResource)
		if err != nil {
			return false
		}
	} else if reqPrefix == "srt" {
		match, err = Match(polResource, reqResource)
		if err != nil {
			return false
		}
	} else {
		match, err = Match(polResource, reqResource)
		if err != nil {
			return false
		}
	}

	fmt.Printf("match: %v\n", match)

	return match
}

func ResourceMatchFunc(args ...interface{}) (interface{}, error) {
	name1 := args[0].(string)
	name2 := args[1].(string)
	name3 := args[2].(string)

	return (bool)(ResourceMatch(name1, name2, name3)), nil
}

func ActionMatch(request string, policy string) bool {
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

func ActionMatchFunc(args ...interface{}) (interface{}, error) {
	name1 := args[0].(string)
	name2 := args[1].(string)

	return (bool)(ActionMatch(name1, name2)), nil
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

func Match(pattern, name string, separators ...rune) (bool, error) {
	g, err := glob.Compile(pattern, separators...)
	if err != nil {
		return false, err
	}

	return g.Match(name), nil
}

func addGroup(e *casbin.Enforcer, name string) error {
	rules := [][]string{}

	rules = append(rules, []string{"role:admin", name, "api:/process/**", "GET|POST|PUT|DELETE"})
	rules = append(rules, []string{"role:admin", name, "processid:*", "CONFIG|PROGRESS|REPORT|METADATA|COMMAND"})
	rules = append(rules, []string{"role:admin", name, "rtmp:" + name + "/*", "PUBLISH|PLAY"})
	rules = append(rules, []string{"role:admin", name, "srt:" + name + "/*", "PUBLISH|PLAY"})
	rules = append(rules, []string{"role:admin", name, "fs:/" + name + "/**", "GET|POST|PUT|DELETE"})
	rules = append(rules, []string{"role:admin", name, "fs:/memfs/" + name + "/**", "GET|POST|PUT|DELETE"})

	rules = append(rules, []string{"role:user", name, "api:/process/**", "GET"})
	rules = append(rules, []string{"role:user", name, "processid:*", "PROGRESS"})
	rules = append(rules, []string{"role:user", name, "rtmp:" + name + "/*", "PLAY"})
	rules = append(rules, []string{"role:user", name, "srt:" + name + "/*", "PLAY"})
	rules = append(rules, []string{"role:user", name, "fs:/" + name + "/**", "GET"})
	rules = append(rules, []string{"role:user", name, "fs:/memfs/" + name + "/**", "GET"})

	rules = append(rules, []string{"role:anonymous", name, "rtmp:" + name + "/*", "PLAY"})
	rules = append(rules, []string{"role:anonymous", name, "srt:" + name + "/*", "PLAY"})
	rules = append(rules, []string{"role:anonymous", name, "fs:/" + name + "/**", "GET"})
	rules = append(rules, []string{"role:anonymous", name, "fs:/memfs/" + name + "/**", "GET"})

	_, err := e.AddPolicies(rules)

	return err
}

func addGroupUser(e *casbin.Enforcer, group, username, role string) error {
	_, err := e.AddGroupingPolicy(username, "role:"+role, group)

	return err
}
