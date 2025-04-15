package replace

import (
	"regexp"
	"strings"

	"github.com/datarhei/core/v16/glob"
	"github.com/datarhei/core/v16/restream/app"
)

type ReplaceFunc func(params map[string]string, config *app.Config, section string) string

type Replacer interface {
	// RegisterReplaceFunc registers a function for replacing for a specific placeholder.
	// If a parameter is not of a placeholder is not present, default values can be provided.
	RegisterReplaceFunc(placeholder string, replacer ReplaceFunc, defaults map[string]string)

	// Replace replaces all occurences of placeholder in str with value. The placeholder is of the
	// form {placeholder}. It is possible to escape a characters in value with \\ by appending a ^
	// and the character to escape to the placeholder name, e.g. {placeholder^:} to escape ":".
	// A placeholder may also have parameters of the form {placeholder,key1=value1,key2=value2}.
	// If the value is an empty string, the registered replacer functions will be searched for that
	// placeholder. If no function is found, the placeholder will be replaced by the empty string.
	// A placeholder name may consist on of the letters a-z and ':'. The placeholder may contain
	// a glob pattern to find the appropriate template.
	Replace(str, placeholder, value string, vars map[string]string, config *app.Config, section string) string

	// Has returns whether the string contains the placeholder
	Has(str, placeholder string) bool
}

type replace struct {
	fn       ReplaceFunc
	defaults map[string]string
}

type replacer struct {
	replacers map[string]replace

	re         *regexp.Regexp
	templateRe *regexp.Regexp
}

// New returns a Replacer
func New() Replacer {
	r := &replacer{
		replacers:  make(map[string]replace),
		re:         regexp.MustCompile(`{([a-z]+(?::[0-9A-Za-z]+)?)(?:\^(.))?(?:,(.*?))?}`),
		templateRe: regexp.MustCompile(`{([a-z:]+)}`),
	}

	return r
}

func (r *replacer) RegisterReplaceFunc(placeholder string, replaceFn ReplaceFunc, defaults map[string]string) {
	r.replacers[placeholder] = replace{
		fn:       replaceFn,
		defaults: defaults,
	}
}

func (r *replacer) Replace(str, placeholder, value string, vars map[string]string, config *app.Config, section string) string {
	str = r.re.ReplaceAllStringFunc(str, func(match string) string {
		matches := r.re.FindStringSubmatch(match)

		if ok, _ := glob.Match(placeholder, matches[1], ':'); !ok {
			return match
		}

		placeholder := matches[1]

		// We need a copy from the value
		v := value
		var repl replace = replace{
			fn: func(map[string]string, *app.Config, string) string { return v },
		}

		if len(v) == 0 {
			// Check for a registered template
			t, ok := r.replacers[placeholder]
			if ok {
				repl = t
			}
		}

		params := r.parseParameters(matches[3], vars, repl.defaults)

		v = repl.fn(params, config, section)

		if len(matches[2]) != 0 {
			// If there's a character to escape, we also have to escape the
			// escape character, but only if it is different from the character
			// to escape.
			if matches[2] != "\\" {
				v = strings.ReplaceAll(v, "\\", "\\\\\\")
			}
			v = strings.ReplaceAll(v, matches[2], "\\\\"+matches[2])
		}

		return strings.Replace(match, match, v, 1)
	})

	return str
}

func (r *replacer) Has(str, placeholder string) bool {
	matches := r.re.FindAllStringSubmatch(str, -1)
	if len(matches) == 0 {
		return false
	}

	for _, match := range matches {
		if ok, _ := glob.Match(placeholder, match[1], ':'); ok {
			return true
		}

		if match[1] == placeholder {
			return true
		}
	}

	return false
}

// parseParameters parses the parameters of a placeholder. The params string is a comma-separated
// string of key=value pairs. The key and values can be escaped with an \.
// The provided defaults will be used as basis. Any parsed key/value from the params might overwrite
// the default value. Any variables in the values will be replaced by their value from the
// vars parameter.
func (r *replacer) parseParameters(params string, vars map[string]string, defaults map[string]string) map[string]string {
	reSpace := regexp.MustCompile(`^\s+`)

	p := make(map[string]string)

	if len(params) == 0 && len(defaults) == 0 {
		return p
	}

	// Copy the defaults
	for key, value := range defaults {
		for name, v := range vars {
			value = strings.ReplaceAll(value, "$"+name, v)
		}

		p[key] = value
	}

	// taken from net/url.ParseQuery
	for params != "" {
		var key string
		key, params, _ = cut([]rune(params), ',')
		key = reSpace.ReplaceAllString(key, "") // Remove all leading spaces

		if key == "" {
			continue
		}

		key, value, _ := cut([]rune(key), '=')

		value = unescape([]rune(value))

		for name, v := range vars {
			value = strings.ReplaceAll(value, "$"+name, v)
		}

		p[key] = value
	}

	return p
}

func cut(s []rune, sep rune) (before, after string, found bool) {
	if i := index(s, sep); i >= 0 {
		return string(s[:i]), string(s[i+1:]), true
	}

	return string(s), "", false
}

func index(s []rune, sep rune) int {
	ignoreSep := false
	for i, c := range s {
		if c == rune('\\') {
			ignoreSep = true
			continue
		}

		if c == sep && !ignoreSep {
			return i
		}

		ignoreSep = false
	}

	return -1
}

func unescape(e []rune) string {
	r := make([]rune, len(e))

	ignore := false
	i := 0
	for _, c := range e {
		if c == rune('\\') && !ignore {
			ignore = true
			continue
		}

		ignore = false

		r[i] = c
		i++
	}

	return string(r[:i])
}
