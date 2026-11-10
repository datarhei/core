package strftime

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type compileHandler interface {
	handleVerbatim(string)
	handleSpec(Appender)
}

// compile, and create an appender list
type appenderListBuilder struct {
	list *combiningAppend
}

func (alb *appenderListBuilder) handleVerbatim(s string) {
	alb.list.Append(Verbatim(s))
}

func (alb *appenderListBuilder) handleSpec(a Appender) {
	alb.list.Append(a)
}

// compile, and execute the appenders on the fly
type appenderExecutor struct {
	t   time.Time
	dst []byte
}

// handleVerbatim appends the static text directly, avoiding the heap
// allocation that boxing it into a verbatimw Appender would incur on
// this per-call compile path.
func (ae *appenderExecutor) handleVerbatim(s string) {
	ae.dst = append(ae.dst, s...)
}

func (ae *appenderExecutor) handleSpec(a Appender) {
	ae.dst = a.Append(ae.dst, ae.t)
}

func compile(handler compileHandler, p string, ds SpecificationSet) error {
	for l := len(p); l > 0; l = len(p) {
		i := strings.IndexByte(p, '%')
		if i < 0 {
			handler.handleVerbatim(p)
			// this is silly, but I don't trust break keywords when there's a
			// possibility of this piece of code being rearranged
			p = p[l:]
			continue
		}
		if i == l-1 {
			return errors.New(`stray % at the end of pattern`)
		}

		// we found a '%'. we need the next byte to decide what to do next
		// we already know that i < l - 1
		// everything up to the i is verbatim
		if i > 0 {
			handler.handleVerbatim(p[:i])
			p = p[i:]
		}

		// An optional '-' (glibc) or '#' (Windows) flag between the '%' and
		// the conversion specifier suppresses padding on numeric fields.
		specIdx := 1
		var noPad bool
		if c := p[1]; c == '-' || c == '#' {
			if len(p) < 3 {
				return errors.New(`stray % at the end of pattern`)
			}
			noPad = true
			specIdx = 2
		}

		specification, err := ds.Lookup(p[specIdx])
		if err != nil {
			return fmt.Errorf("pattern compilation failed: %w", err)
		}

		if noPad && !isStandardNonNumericSpecification(p[specIdx]) {
			specification = unpadded{inner: specification}
		}

		handler.handleSpec(specification)
		p = p[specIdx+1:]
	}
	return nil
}

func getSpecificationSetFor(options ...Option) (SpecificationSet, error) {
	ds := defaultSpecificationSet
	var extraSpecifications []*optSpecificationPair
	var locale Locale
	for _, option := range options {
		switch option.Name() {
		case optSpecificationSet:
			if v, ok := option.Value().(SpecificationSet); ok {
				ds = v
			}
		case optSpecification:
			if v, ok := option.Value().(*optSpecificationPair); ok {
				extraSpecifications = append(extraSpecifications, v)
			}
		case optLocale:
			if v, ok := option.Value().(Locale); ok {
				locale = v
			}
		}
	}

	if locale != nil || len(extraSpecifications) > 0 {
		// If ds is immutable, we're going to need to create a new
		// one. oh what a waste!
		if raw, ok := ds.(*specificationSet); ok && !raw.mutable {
			ds = NewSpecificationSet()
		}
		// Apply the locale first so an explicit WithSpecification can still
		// override an individual specifier.
		if locale != nil {
			if err := applyLocale(ds, locale); err != nil {
				return nil, err
			}
		}
		for _, v := range extraSpecifications {
			if err := ds.Set(v.name, v.appender); err != nil {
				return nil, err
			}
		}
	}
	return ds, nil
}

var fmtAppendExecutorPool = sync.Pool{
	New: func() any {
		var h appenderExecutor
		h.dst = make([]byte, 0, 32)
		return &h
	},
}

func getFmtAppendExecutor() *appenderExecutor {
	e, _ := fmtAppendExecutorPool.Get().(*appenderExecutor)
	return e
}

func releasdeFmtAppendExecutor(v *appenderExecutor) {
	// TODO: should we discard the buffer if it's too long?
	v.dst = v.dst[:0]
	fmtAppendExecutorPool.Put(v)
}

// formatCacheLimit caps the number of distinct patterns Format will keep
// compiled. The bound keeps memory usage predictable even when patterns are
// derived from untrusted input; once it is reached, additional patterns are
// formatted on the fly without being cached.
const formatCacheLimit = 1024

var (
	formatCache    sync.Map // pattern string -> *Strftime
	formatCacheLen atomic.Int64
)

// cachedStrftime returns a compiled Strftime for the default specification
// set, reusing a previously compiled one when possible. The boolean result is
// false (with no error) when the cache is full and the pattern was not already
// cached, so the caller can fall back to compiling on the fly.
func cachedStrftime(p string) (*Strftime, bool, error) {
	if v, ok := formatCache.Load(p); ok {
		f, _ := v.(*Strftime)
		return f, true, nil
	}
	if formatCacheLen.Load() >= formatCacheLimit {
		return nil, false, nil
	}

	f, err := New(p)
	if err != nil {
		return nil, false, err
	}
	if actual, loaded := formatCache.LoadOrStore(p, f); loaded {
		cached, _ := actual.(*Strftime)
		return cached, true, nil
	}
	formatCacheLen.Add(1)
	return f, true, nil
}

// Format takes the format `s` and the time `t` to produce the
// format date/time.
//
// When called without options, compiled patterns are cached (up to an
// internal limit) so that repeated calls with the same pattern avoid
// recompilation. Calls that pass options always compile on the fly.
//
// If you know beforehand that you will be reusing the pattern
// within your application, consider creating a `Strftime` object
// and reusing it.
func Format(p string, t time.Time, options ...Option) (string, error) {
	if len(options) == 0 {
		f, ok, err := cachedStrftime(p)
		if err != nil {
			return "", fmt.Errorf("failed to compile format: %w", err)
		}
		if ok {
			return f.FormatString(t), nil
		}
		// cache is full: fall through and format on the fly
	}

	ds, err := getSpecificationSetFor(options...)
	if err != nil {
		return "", fmt.Errorf("failed to get specification set: %w", err)
	}
	h := getFmtAppendExecutor()
	defer releasdeFmtAppendExecutor(h)

	h.t = t
	if err := compile(h, p, ds); err != nil {
		return "", fmt.Errorf("failed to compile format: %w", err)
	}

	return string(h.dst), nil
}

// Strftime is the object that represents a compiled strftime pattern
type Strftime struct {
	pattern  string
	compiled appenderList
}

// New creates a new Strftime object. If the compilation fails, then
// an error is returned in the second argument.
func New(p string, options ...Option) (*Strftime, error) {
	// TODO: this may be premature optimization
	ds, err := getSpecificationSetFor(options...)
	if err != nil {
		return nil, fmt.Errorf("failed to get specification set: %w", err)
	}

	var h appenderListBuilder
	h.list = &combiningAppend{}

	if err := compile(&h, p, ds); err != nil {
		return nil, fmt.Errorf("failed to compile format: %w", err)
	}

	return &Strftime{
		pattern:  p,
		compiled: h.list.list,
	}, nil
}

// Pattern returns the original pattern string
func (f *Strftime) Pattern() string {
	return f.pattern
}

// Format takes the destination `dst` and time `t`. It formats the date/time
// using the pre-compiled pattern, and outputs the results to `dst`
func (f *Strftime) Format(dst io.Writer, t time.Time) error {
	const bufSize = 64
	var b []byte
	bufLen := len(f.pattern) + 10
	if bufLen < bufSize {
		var buf [bufSize]byte
		b = buf[:0]
	} else {
		b = make([]byte, 0, bufLen)
	}
	if _, err := dst.Write(f.format(b, t)); err != nil {
		return err
	}
	return nil
}

// FormatBuffer is equivalent to Format, but appends the result directly to
// supplied slice dst, returning the updated slice. This avoids any internal
// memory allocation.
func (f *Strftime) FormatBuffer(dst []byte, t time.Time) []byte {
	return f.format(dst, t)
}

// Dump outputs the internal structure of the formatter, for debugging purposes.
// Please do NOT assume the output format to be fixed: it is expected to change
// in the future.
func (f *Strftime) Dump(out io.Writer) {
	f.compiled.dump(out)
}

func (f *Strftime) format(b []byte, t time.Time) []byte {
	for _, w := range f.compiled {
		b = w.Append(b, t)
	}
	return b
}

// FormatString takes the time `t` and formats it, returning the
// string containing the formated data.
func (f *Strftime) FormatString(t time.Time) string {
	const bufSize = 64
	var b []byte
	bufLen := len(f.pattern) + 10
	if bufLen < bufSize {
		var buf [bufSize]byte
		b = buf[:0]
	} else {
		b = make([]byte, 0, bufLen)
	}
	return string(f.format(b, t))
}
