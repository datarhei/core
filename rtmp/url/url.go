package url

import (
	"net/url"
	"path/filepath"
	"strings"
)

// GetToken returns the path without the token and the token found in the URL and whether
// it was found in the path. If the token was part of the path, the token is removed from
// the path. The token in the query string takes precedence. The token in the path is
// assumed to be the last path element.
func GetToken(u *url.URL) (string, string, bool) {
	q := u.Query()
	if q.Has("token") {
		// The token was in the query. Return the unmomdified path and the token.
		return u.Path, q.Get("token"), false
	}

	pathElements := SplitPath(u.EscapedPath())
	nPathElements := len(pathElements)

	if nPathElements <= 1 {
		return u.Path, "", false
	}

	rawPath := "/" + strings.Join(pathElements[:nPathElements-1], "/")
	rawToken := pathElements[nPathElements-1]

	path, err := url.PathUnescape(rawPath)
	if err != nil {
		path = rawPath
	}

	token, err := url.PathUnescape(rawToken)
	if err != nil {
		token = rawToken
	}

	// Return the path without the token
	return path, token, true
}

func SplitPath(path string) []string {
	pathElements := strings.Split(filepath.Clean(path), "/")

	if len(pathElements) == 0 {
		return pathElements
	}

	if len(pathElements[0]) == 0 {
		pathElements = pathElements[1:]
	}

	return pathElements
}

func RemovePathPrefix(path, prefix string) (string, string) {
	prefix = filepath.Join("/", prefix)
	return filepath.Join("/", strings.TrimPrefix(path, prefix+"/")), prefix
}
