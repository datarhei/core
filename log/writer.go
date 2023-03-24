package log

import (
	"container/ring"
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/lithammer/shortuuid/v4"
	"github.com/mattn/go-isatty"
)

type Writer interface {
	Write(e *Event) error
	Close()
}

type jsonWriter struct {
	writer    io.Writer
	level     Level
	formatter Formatter
}

func NewJSONWriter(w io.Writer, level Level) Writer {
	writer := &jsonWriter{
		writer:    w,
		level:     level,
		formatter: NewJSONFormatter(),
	}

	return NewSyncWriter(writer)
}

func (w *jsonWriter) Write(e *Event) error {
	if w.level < e.Level || e.Level == Lsilent {
		return nil
	}

	_, err := w.writer.Write(w.formatter.Bytes(e))

	return err
}

func (w *jsonWriter) Close() {}

type consoleWriter struct {
	writer    io.Writer
	level     Level
	formatter Formatter
}

func NewConsoleWriter(w io.Writer, level Level, useColor bool) Writer {
	writer := &consoleWriter{
		writer: w,
		level:  level,
	}

	color := useColor

	if color {
		if w, ok := w.(*os.File); ok {
			if !isatty.IsTerminal(w.Fd()) && !isatty.IsCygwinTerminal(w.Fd()) {
				color = false
			}
		} else {
			color = false
		}
	}

	writer.formatter = NewConsoleFormatter(color)

	return NewSyncWriter(writer)
}

func (w *consoleWriter) Write(e *Event) error {
	if w.level < e.Level || e.Level == Lsilent {
		return nil
	}

	_, err := w.writer.Write(w.formatter.Bytes(e))

	return err
}

func (w *consoleWriter) Close() {}

type topicWriter struct {
	writer Writer
	topics map[string]struct{}
}

func NewTopicWriter(writer Writer, topics []string) Writer {
	w := &topicWriter{
		writer: writer,
		topics: make(map[string]struct{}),
	}

	for _, topic := range topics {
		topic = strings.ToLower(topic)
		w.topics[topic] = struct{}{}
	}

	return w
}

func (w *topicWriter) Write(e *Event) error {
	if len(w.topics) > 0 {
		topic := strings.ToLower(e.Component)
		if _, ok := w.topics[topic]; !ok {
			return nil
		}
	}

	err := w.writer.Write(e)

	return err
}

func (w *topicWriter) Close() {
	w.writer.Close()
}

type levelRewriter struct {
	writer Writer
	rules  []levelRewriteRule
}

type LevelRewriteRule struct {
	Level     Level
	Component string
	Match     map[string]string
}

type levelRewriteRule struct {
	level     Level
	component string
	match     map[string]*regexp.Regexp
}

func NewLevelRewriter(writer Writer, rules []LevelRewriteRule) Writer {
	w := &levelRewriter{
		writer: writer,
	}

	for _, rule := range rules {
		r := levelRewriteRule{
			level:     rule.Level,
			component: rule.Component,
			match:     make(map[string]*regexp.Regexp),
		}

		for k, v := range rule.Match {
			re, err := regexp.Compile(v)
			if err != nil {
				continue
			}
			r.match[k] = re
		}

		w.rules = append(w.rules, r)
	}

	return w
}

func (w *levelRewriter) Write(e *Event) error {
rules:
	for _, r := range w.rules {
		if e.Component != r.component {
			continue
		}

		for k, re := range r.match {
			value, ok := e.Data[k]
			if !ok {
				continue rules
			}

			switch value := value.(type) {
			case string:
				if !re.MatchString(value) {
					continue rules
				}
			}
		}

		e.Level = r.level
	}

	return w.writer.Write(e)
}

func (w *levelRewriter) Close() {
	w.writer.Close()
}

type syncWriter struct {
	mu     sync.Mutex
	writer Writer
}

func NewSyncWriter(writer Writer) Writer {
	return &syncWriter{
		writer: writer,
	}
}

func (w *syncWriter) Write(e *Event) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.writer.Write(e)
}

func (w *syncWriter) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.writer.Close()
}

type multiWriter struct {
	writer []Writer
}

func NewMultiWriter(writer ...Writer) Writer {
	mw := &multiWriter{}

	mw.writer = append(mw.writer, writer...)

	return mw
}

func (w *multiWriter) Write(e *Event) error {
	for _, writer := range w.writer {
		if err := writer.Write(e); err != nil {
			return err
		}
	}

	return nil
}

func (w *multiWriter) Close() {
	for _, writer := range w.writer {
		writer.Close()
	}
}

type BufferWriter interface {
	Writer
	Events() []*Event
}

type bufferWriter struct {
	lines *ring.Ring
	lock  sync.RWMutex
	level Level
}

func NewBufferWriter(level Level, lines int) BufferWriter {
	b := &bufferWriter{
		level: level,
	}

	if lines > 0 {
		b.lines = ring.New(lines)
	}

	return b
}

func (w *bufferWriter) Write(e *Event) error {
	if w.level < e.Level || e.Level == Lsilent {
		return nil
	}

	w.lock.Lock()
	defer w.lock.Unlock()

	if w.lines != nil {
		w.lines.Value = e.clone()
		w.lines = w.lines.Next()
	}

	return nil
}

func (w *bufferWriter) Close() {
	w.lock.Lock()
	defer w.lock.Unlock()

	w.lines = nil
}

func (w *bufferWriter) Events() []*Event {
	var lines = []*Event{}

	if w.lines == nil {
		return lines
	}

	w.lock.RLock()
	defer w.lock.RUnlock()

	w.lines.Do(func(l interface{}) {
		if l == nil {
			return
		}

		lines = append(lines, l.(*Event).clone())
	})

	return lines
}

type ChannelWriter interface {
	Writer

	Subscribe() (<-chan Event, func())
}

type channelWriter struct {
	publisher       chan Event
	publisherClosed bool
	publisherLock   sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc

	subscriber     map[string]chan Event
	subscriberLock sync.Mutex
}

func NewChannelWriter() ChannelWriter {
	w := &channelWriter{
		publisher:       make(chan Event, 1024),
		publisherClosed: false,
		subscriber:      make(map[string]chan Event),
	}

	w.ctx, w.cancel = context.WithCancel(context.Background())

	go w.broadcast()

	return w
}

func (w *channelWriter) Write(e *Event) error {
	event := e.clone()
	event.logger = nil

	w.publisherLock.Lock()
	defer w.publisherLock.Unlock()

	if w.publisherClosed {
		return fmt.Errorf("writer is closed")
	}

	select {
	case w.publisher <- *e:
	default:
		return fmt.Errorf("publisher queue full")
	}

	return nil
}

func (w *channelWriter) Close() {
	w.cancel()

	w.publisherLock.Lock()
	close(w.publisher)
	w.publisherClosed = true
	w.publisherLock.Unlock()

	w.subscriberLock.Lock()
	for _, c := range w.subscriber {
		close(c)
	}
	w.subscriber = make(map[string]chan Event)
	w.subscriberLock.Unlock()
}

func (w *channelWriter) Subscribe() (<-chan Event, func()) {
	l := make(chan Event, 1024)

	var id string = ""

	w.subscriberLock.Lock()
	for {
		id = shortuuid.New()
		if _, ok := w.subscriber[id]; !ok {
			w.subscriber[id] = l
			break
		}
	}
	w.subscriberLock.Unlock()

	unsubscribe := func() {
		w.subscriberLock.Lock()
		delete(w.subscriber, id)
		w.subscriberLock.Unlock()
	}

	return l, unsubscribe
}

func (w *channelWriter) broadcast() {
	for {
		select {
		case <-w.ctx.Done():
			return
		case e := <-w.publisher:
			w.subscriberLock.Lock()
			for _, c := range w.subscriber {
				pp := e.clone()

				select {
				case c <- *pp:
				default:
				}
			}
			w.subscriberLock.Unlock()
		}
	}
}
