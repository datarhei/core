package token

import "strings"

func Marshal(username, token string) string {
	username = strings.ReplaceAll(username, ":", "::")

	return username + ":" + token
}

// Unmarshal splits a username/token combination into a username and
// token. If the input doesn't contain a username, the whole input
// is returned as token.
func Unmarshal(usertoken string) (string, string) {
	r := []rune(usertoken)

	count := 0
	index := -1
	for i, ru := range r {
		if ru == ':' {
			count++
			continue
		}

		if count > 0 && count%2 != 0 {
			index = i - 1
			break
		}

		count = 0
	}

	if index == -1 {
		return "", usertoken
	}

	before, after := r[:index], r[index+1:]

	username := string(before)
	token := string(after)

	username = strings.ReplaceAll(username, "::", ":")

	return username, token
}
