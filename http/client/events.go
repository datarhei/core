package client

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"

	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/mem"
)

func (r *restclient) LogEvents(ctx context.Context, filters api.LogEventFilters) (<-chan api.LogEventRaw, error) {
	buf := mem.Get()
	defer mem.Put(buf)

	e := json.NewEncoder(buf)
	e.Encode(filters)

	header := make(http.Header)
	header.Set("Accept", "application/x-json-stream")

	stream, err := r.stream(ctx, "POST", "/v3/events", nil, header, "application/json", buf.Reader())
	if err != nil {
		return nil, err
	}

	channel := make(chan api.LogEventRaw, 128)

	go func(stream io.ReadCloser, ch chan<- api.LogEventRaw) {
		defer stream.Close()
		defer close(channel)

		scanner := bufio.NewScanner(stream)
		scanner.Split(bufio.ScanLines)

		for scanner.Scan() {
			data := bytes.Clone(scanner.Bytes())

			ch <- data
		}
		/*
			decoder := json.NewDecoder(stream)

			for decoder.More() {
				var event api.LogEventRaw
				if err := decoder.Decode(&event); err != nil {
					return
				}

				ch <- event
			}
		*/
	}(stream, channel)

	return channel, nil
}

func (r *restclient) MediaEvents(ctx context.Context, storage, pattern string) (<-chan api.MediaEvent, error) {
	header := make(http.Header)
	header.Set("Accept", "application/x-json-stream")
	header.Set("Connection", "close")

	query := &url.Values{}
	query.Set("glob", pattern)

	stream, err := r.stream(ctx, "POST", "/v3/events/media/"+url.PathEscape(storage), query, header, "", nil)
	if err != nil {
		return nil, err
	}

	channel := make(chan api.MediaEvent, 128)

	go func(stream io.ReadCloser, ch chan<- api.MediaEvent) {
		defer stream.Close()
		defer close(channel)

		decoder := json.NewDecoder(stream)

		for decoder.More() {
			var event api.MediaEvent
			if err := decoder.Decode(&event); err == io.EOF {
				return
			} else if err != nil {
				event.Action = "error"
				event.Name = err.Error()
			}

			// Don't emit keepalives
			if event.Action == "keepalive" {
				continue
			}

			ch <- event

			if event.Action == "" || event.Action == "error" {
				return
			}
		}
	}(stream, channel)

	return channel, nil
}

func (r *restclient) ProcessEvents(ctx context.Context, filters api.ProcessEventFilters) (<-chan api.ProcessEventRaw, error) {
	buf := mem.Get()
	defer mem.Put(buf)

	e := json.NewEncoder(buf)
	e.Encode(filters)

	header := make(http.Header)
	header.Set("Accept", "application/x-json-stream")

	stream, err := r.stream(ctx, "POST", "/v3/events/process", nil, header, "application/json", buf.Reader())
	if err != nil {
		return nil, err
	}

	channel := make(chan api.ProcessEventRaw, 128)

	go func(stream io.ReadCloser, ch chan<- api.ProcessEventRaw) {
		defer stream.Close()
		defer close(channel)

		scanner := bufio.NewScanner(stream)
		scanner.Split(bufio.ScanLines)

		for scanner.Scan() {
			data := bytes.Clone(scanner.Bytes())

			ch <- data
		}

		/*
			decoder := json.NewDecoder(io.TeeReader(stream, os.Stdout))

			for decoder.More() {
				var event api.ProcessEventRaw
				if err := decoder.Decode(&event); err != nil {
					return
				}

				ch <- event
			}
		*/
	}(stream, channel)

	return channel, nil
}
