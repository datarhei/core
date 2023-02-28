package replace

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/datarhei/core/v16/glob"
	"github.com/datarhei/core/v16/restream/app"
)

type ReplaceFunc func(params map[string]string, config *app.Config, section string) string

type Replacer interface {
	// RegisterTemplate registers a template for a specific placeholder. Template
	// may contain placeholders as well of the form {name}. They will be replaced
	// by the parameters of the placeholder (see Replace). If a parameter is not of
	// a template is not present, default values can be provided.

	// RegisterTemplateFunc does the same as RegisterTemplate, but the template
	// is returned by the template function.

	RegisterReplaceFunc(placeholder string, replacer ReplaceFunc, defaults map[string]string)

	// Replace replaces all occurences of placeholder in str with value. The placeholder is of the
	// form {placeholder}. It is possible to escape a characters in value with \\ by appending a ^
	// and the character to escape to the placeholder name, e.g. {placeholder^:} to escape ":".
	// A placeholder may also have parameters of the form {placeholder,key1=value1,key2=value2}.
	// If the value has placeholders itself (see RegisterTemplate), they will be replaced by
	// the value of the corresponding key in the parameters.
	// If the value is an empty string, the registered templates will be searched for that
	// placeholder. If no template is found, the placeholder will be replaced by the empty string.
	// A placeholder name may consist on of the letters a-z and ':'. The placeholder may contain
	// a glob pattern to find the appropriate template.
	Replace(str, placeholder, value string, vars map[string]string, config *app.Config, section string) string
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
		re:         regexp.MustCompile(`{([a-z:]+)(?:\^(.))?(?:,(.*?))?}`),
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

		params := r.parseParametes(matches[3], vars, repl.defaults)

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

func (r *replacer) parseParametes(params string, vars map[string]string, defaults map[string]string) map[string]string {
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
		key, params, _ = strings.Cut(params, ",")
		if key == "" {
			continue
		}

		key, value, _ := strings.Cut(key, "=")
		key, err := url.QueryUnescape(key)
		if err != nil {
			continue
		}

		value, err = url.QueryUnescape(value)
		if err != nil {
			continue
		}

		for name, v := range vars {
			value = strings.ReplaceAll(value, "$"+name, v)
		}

		p[key] = value
	}

	return p
}

// compileTemplate fills in the placeholder in the template with the values from the params
// string. The placeholders in the template are delimited by {} and their name may only
// contain the letters a-z. The params string is a comma-separated string of key=value pairs.
// Example: the template is "Hello {who}!", the params string is "who=World". The key is the
// placeholder name and will be replaced with the value. The resulting string is "Hello World!".
// If a placeholder name is not present in the params string, it will not be replaced. The key
// and values can be escaped as in net/url.QueryEscape.
