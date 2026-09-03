// Package session is a HLS session middleware for Gin
package session

import (
	"bufio"
	"bytes"
	"io"
	"math"
	"net/http"
	"net/url"
	urlpath "path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/datarhei/core/v16/mem"
	"github.com/datarhei/core/v16/net"
	"github.com/lithammer/shortuuid/v5"

	"github.com/labstack/echo/v4"
)

func (h *handler) handleHLS(c echo.Context, ctxuser string, data map[string]interface{}, next echo.HandlerFunc) error {
	req := c.Request()

	switch req.Method {
	case "PUT", "POST":
		return h.handleHLSIngress(c, ctxuser, data, next)
	case "GET", "HEAD":
		return h.handleHLSEgress(c, ctxuser, data, next)
	}

	return next(c)
}

func (h *handler) handleHLSIngress(c echo.Context, _ string, data map[string]interface{}, next echo.HandlerFunc) error {
	req := c.Request()
	path := req.URL.Path

	isM3U8 := strings.HasSuffix(path, ".m3u8")
	isSegment := strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".mp4") || strings.HasSuffix(path, ".m4s")

	if isM3U8 {
		// Read out the path of the .ts files and look them up in the ts-map.
		// Add it as ingress for the respective "sessionId". The "sessionId" is the .m3u8 file name.
		reader := req.Body
		r := &segmentReader{
			reader: req.Body,
			buffer: mem.Get(),
		}
		req.Body = r

		defer func() {
			req.Body = reader

			if r.size == 0 {
				mem.Put(r.buffer)
				return
			}

			if !h.hlsIngressCollector.IsKnownSession(path) {
				ip, _ := net.AnonymizeIPString(c.RealIP())

				// Register a new session
				reference := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
				h.hlsIngressCollector.RegisterAndActivate(path, reference, path, ip)
				h.hlsIngressCollector.Extra(path).SetAll(data)
			}

			buffer := mem.Get()
			h.hlsIngressCollector.Ingress(path, headerSize(req.Header, buffer))
			h.hlsIngressCollector.Ingress(path, r.size)
			mem.Put(buffer)

			segments := r.getSegments(urlpath.Dir(path))

			if len(segments) != 0 {
				h.lock.Lock()
				for _, seg := range segments {
					if size, ok := h.rxsegments[seg]; ok {
						// Update ingress value
						h.hlsIngressCollector.Ingress(path, size)
						delete(h.rxsegments, seg)
					}
				}
				h.lock.Unlock()
			}

			mem.Put(r.buffer)
		}()
	} else if isSegment {
		// Get the size of the .ts file and store it in the ts-map for later use.
		reader := req.Body
		r := &bodysizeReader{
			reader: req.Body,
		}
		req.Body = r

		defer func() {
			req.Body = reader

			if r.size != 0 {
				buffer := mem.Get()
				h.lock.Lock()
				h.rxsegments[path] = r.size + headerSize(req.Header, buffer)
				h.lock.Unlock()
				mem.Put(buffer)
			}
		}()
	}

	return next(c)
}

func (h *handler) handleHLSEgress(c echo.Context, _ string, data map[string]interface{}, next echo.HandlerFunc) error {
	req := c.Request()
	res := c.Response()

	if !h.hlsEgressCollector.IsCollectableIP(c.RealIP()) {
		return next(c)
	}

	path := req.URL.Path
	sessionID := c.QueryParam("session")

	isM3U8 := strings.HasSuffix(path, ".m3u8")
	isSegment := strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".mp4") || strings.HasSuffix(path, ".m4s")

	rewrite := false

	if isM3U8 {
		if !h.hlsEgressCollector.IsKnownSession(sessionID) {
			if h.hlsEgressCollector.IsSessionsExceeded() {
				return echo.NewHTTPError(509, "Number of sessions exceeded")
			}

			streamBitrate := h.hlsIngressCollector.SessionTopIngressBitrate(path) * 2.0 // Multiply by 2 to cover the initial peak
			maxBitrate := h.hlsEgressCollector.MaxEgressBitrate()

			if maxBitrate > 0.0 {
				currentBitrate := h.hlsEgressCollector.CompanionTopEgressBitrate() * 1.15

				// Add the new session's top bitrate to the ingress top bitrate
				resultingBitrate := currentBitrate + streamBitrate

				if resultingBitrate >= maxBitrate {
					return echo.NewHTTPError(509, "Bitrate limit exceeded")
				}
			}

			if len(sessionID) != 0 {
				if !h.reSessionID.MatchString(sessionID) {
					return echo.NewHTTPError(http.StatusForbidden)
				}

				referrer := req.Header.Get("Referer")
				if u, err := url.Parse(referrer); err == nil {
					referrer = u.Host
				}

				reference := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

				// Register a new session
				h.hlsEgressCollector.Register(sessionID, reference, path, referrer)
				h.hlsEgressCollector.Extra(sessionID).SetAll(data)

				// Give the new session an initial top bitrate
				h.hlsEgressCollector.SessionSetTopEgressBitrate(sessionID, streamBitrate)
			}
		}

		// Remove any Range request headers, because the rewrite will mess up any lengths.
		req.Header.Del("Range")
		req.Header.Del("If-Range")

		rewrite = true
	}

	var rewriter *sessionRewriter

	// Keep the current writer for later
	writer := res.Writer

	if rewrite {
		// Put the session rewriter in the middle. This will collect
		// the data that we need to rewrite.
		rewriter = &sessionRewriter{
			ResponseWriter: res.Writer,
			buffer:         mem.Get(),
		}

		res.Writer = rewriter
	}

	start := time.Now()

	err := next(c)

	// Restore the original writer
	res.Writer = writer

	if err != nil {
		return err
	}

	duration := time.Since(start)

	variants := []variant{}
	segments := map[string]hlsSegment{}

	if rewrite {
		if res.Status < 200 || res.Status >= 300 {
			res.Write(rewriter.buffer.Bytes())
			mem.Put(rewriter.buffer)
			return nil
		}

		buffer := mem.Get()

		// Rewrite the data before sending it to the client
		newSessionID := rewriter.rewriteHLS(sessionID, c.Request().URL, buffer)
		if newSessionID != sessionID {
			sessionID = newSessionID

			streamBitrate := h.hlsIngressCollector.SessionTopIngressBitrate(path) * 2.0 // Multiply by 2 to cover the initial peak

			// Create new session
			referrer := req.Header.Get("Referer")
			if u, err := url.Parse(referrer); err == nil {
				referrer = u.Host
			}

			reference := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

			// Register a new session
			h.hlsEgressCollector.Register(sessionID, reference, path, referrer)
			h.hlsEgressCollector.Extra(sessionID).SetAll(data)

			// Give the new session an initial top bitrate
			h.hlsEgressCollector.SessionSetTopEgressBitrate(sessionID, streamBitrate)
		}

		variants = parseVariants(path, buffer)
		segments = parseSegments(buffer)

		res.Header().Set("Cache-Control", "private")
		res.Write(buffer.Bytes())

		mem.Put(buffer)
		mem.Put(rewriter.buffer)
	}

	if len(sessionID) == 0 {
		return nil
	}

	var sdata *HLSSessionData = nil
	if x, ok := h.hlsEgressCollector.UserData(sessionID).Get("hlsstats"); ok {
		if data, ok := x.(*HLSSessionData); ok {
			sdata = data
		}
	}

	if sdata == nil {
		sdata = newHLSSessionData()
	}

	if len(sdata.Variants) == 0 {
		if len(variants) != 0 {
			for _, v := range variants {
				sdata.Variants[v.file] = HLSSessionVariant{
					Active:     false,
					Switches:   0,
					Bandwidth:  v.bandwidth,
					Resolution: v.resolution,
					Codecs:     v.codecs,
				}
			}
		}
	}

	if len(variants) != 0 {
		// This is a master file. No further processing needed
		h.hlsEgressCollector.UserData(sessionID).Set("hlsstats", sdata)
		return nil
	}

	if isM3U8 {
		if variant, ok := sdata.Variants[path]; ok {
			for key, variant := range sdata.Variants {
				if key == path {
					continue
				}

				variant.Active = false
				sdata.Variants[key] = variant
			}

			if !variant.Active {
				variant.Active = true
				variant.Switches++
			}

			sdata.Variants[path] = variant
		}

		sdata.Segments.segments = mergeSegments(sdata.Segments.segments, segments)
	}

	sdata.HTTPStatus[res.Status]++

	if isSegment {
		path = filepath.Base(path)

		sdata.Segments.Last = time.Now()
		sdata.Segments.Requested++
		segment, ok := sdata.Segments.segments[path]
		if ok {
			if segment.sequence-sdata.Segments.lastSequence > 1 {
				sdata.Segments.SequenceGaps++
			}

			sdata.Segments.lastSequence = segment.sequence

			if segment.requested {
				sdata.Segments.Retries++
			} else {
				segment.requested = true
				sdata.Segments.segments[path] = segment
			}

			if duration > segment.duration {
				sdata.Segments.TooSlow++
			}
		} else {
			if res.Status > 400 {
				sdata.Segments.Failed++
			} else {
				sdata.Segments.TooLate++
			}
		}

		bitrate := float64(res.Size) * 8 / duration.Seconds()
		if bitrate < sdata.Bandwidth.Min {
			sdata.Bandwidth.Min = bitrate
		}

		if bitrate > sdata.Bandwidth.Max {
			sdata.Bandwidth.Max = bitrate
		}

		sdata.Bandwidth.Avg = sdata.Bandwidth.Avg*0.85 + bitrate*0.15
	}

	h.hlsEgressCollector.UserData(sessionID).Set("hlsstats", sdata)

	if isM3U8 || isSegment {
		if res.Status >= 200 && res.Status < 300 {
			// Collect how many bytes we've written in this session
			buffer := mem.Get()
			h.hlsEgressCollector.Egress(sessionID, headerSize(res.Header(), buffer))
			h.hlsEgressCollector.Egress(sessionID, res.Size)
			mem.Put(buffer)

			if isSegment {
				// Activate the session. If the session is already active, this is a noop
				h.hlsEgressCollector.Activate(sessionID)
			}
		}
	}

	return nil
}

func mergeSegments(have, fresh map[string]hlsSegment) map[string]hlsSegment {
	lowestListedSequence := uint64(math.MaxUint64)

	// Add the new segments to the map
	for path, s := range fresh {
		if s.sequence < lowestListedSequence {
			lowestListedSequence = s.sequence
		}

		if _, ok := have[path]; ok {
			continue
		}

		have[path] = s
	}

	// Remove all segments that have a lower sequence than
	// the lowest listed sequence.
	for path, s := range have {
		if s.sequence < lowestListedSequence {
			delete(have, path)
		}
	}

	return have
}

type hlsSegment struct {
	duration  time.Duration
	sequence  uint64
	requested bool
}

type HLSSessionVariant struct {
	Active     bool   `json:"active"`
	Switches   uint64 `json:"switches"`
	Bandwidth  uint64 `json:"bandwidth_bits"`
	Resolution string `json:"resolution"`
	Codecs     string `json:"codecs"`
}

type HLSSessionData struct {
	Variants map[string]HLSSessionVariant `json:"hls_variants"`
	Segments struct {
		segments     map[string]hlsSegment
		lastSequence uint64
		Requested    uint64    `json:"requests"`
		Failed       uint64    `json:"failed"`
		TooSlow      uint64    `json:"too_slow"`
		Retries      uint64    `json:"retries"`
		TooLate      uint64    `json:"too_late"`
		SequenceGaps uint64    `json:"sequence_gaps"`
		Last         time.Time `json:"last"`
	} `json:"hls_segments"`
	HTTPStatus map[int]uint64 `json:"http_status"`
	Bandwidth  struct {
		Min float64 `json:"min"`
		Max float64 `json:"max"`
		Avg float64 `json:"avg"`
	} `json:"bandwidth_tx_bits"`
}

func newHLSSessionData() *HLSSessionData {
	data := &HLSSessionData{}

	data.Variants = map[string]HLSSessionVariant{}
	data.Segments.segments = map[string]hlsSegment{}
	data.HTTPStatus = map[int]uint64{}
	data.Bandwidth.Min = math.MaxFloat64

	return data
}

type segmentReader struct {
	reader io.ReadCloser
	buffer *mem.Buffer
	size   int64
}

func (r *segmentReader) Read(b []byte) (int, error) {
	n, err := r.reader.Read(b)
	if n > 0 {
		r.buffer.Write(b[:n])
	}
	r.size += int64(n)

	return n, err
}

func (r *segmentReader) Close() error {
	return r.reader.Close()
}

func (r *segmentReader) getSegments(dir string) []string {
	segments := []string{}

	// Find all segment URLs in the .m3u8
	scanner := bufio.NewScanner(r.buffer.Reader())
	for scanner.Scan() {
		line := scanner.Text()

		// Ignore empty lines
		if len(line) == 0 {
			continue
		}

		// Ignore comments
		if strings.HasPrefix(line, "#") {
			continue
		}

		u, err := url.Parse(line)
		if err != nil {
			// Invalid URL
			continue
		}

		if u.Scheme != "" {
			// Ignore full URLs
			continue
		}

		// Ignore anything that doesn't end in .ts
		if !strings.HasSuffix(u.Path, ".ts") && !strings.HasSuffix(u.Path, ".mp4") && !strings.HasSuffix(u.Path, ".m4s") {
			continue
		}

		path := u.Path

		if !strings.HasPrefix(u.Path, "/") {
			path = urlpath.Join(dir, u.Path)
		}

		segments = append(segments, path)
	}

	return segments
}

type sessionRewriter struct {
	http.ResponseWriter
	buffer *mem.Buffer
}

func (g *sessionRewriter) Write(data []byte) (int, error) {
	// Write the data into internal buffer for later rewrite
	return g.buffer.Write(data)
}

func parseSegments(buffer *mem.Buffer) map[string]hlsSegment {
	var reSequence *regexp.Regexp
	isSegment := false
	segments := map[string]hlsSegment{}
	duration := time.Duration(0)

	scanner := bufio.NewScanner(buffer.Reader())
	for scanner.Scan() {
		byteline := scanner.Bytes()

		if len(byteline) == 0 {
			continue
		}

		if !isSegment {
			after, found := bytes.CutPrefix(byteline, []byte("#EXTINF:"))
			if !found {
				continue
			}

			isSegment = true

			before, _, _ := bytes.Cut(after, []byte(","))

			if f, err := strconv.ParseFloat(string(before), 64); err == nil {
				duration = time.Duration(f * float64(time.Second))
			}
		} else {
			if byteline[0] == '#' {
				continue
			}

			s := hlsSegment{
				duration: duration,
			}

			before, _, _ := bytes.Cut(byteline, []byte("?"))
			name := filepath.Base(string(before))
			if reSequence == nil {
				reSequence = regexp.MustCompile(`([0-9]+)\.`)
			}
			matches := reSequence.FindStringSubmatch(name)
			if len(matches) > 1 {
				if x, err := strconv.ParseUint(matches[1], 10, 64); err == nil {
					s.sequence = x
				}
			}
			segments[name] = s

			isSegment = false
			duration = time.Duration(0)
		}
	}

	return segments
}

type variant struct {
	file       string
	bandwidth  uint64
	resolution string
	codecs     string
}

func parseVariants(path string, buffer *mem.Buffer) []variant {
	isVariant := false
	var vari variant
	variants := []variant{}

	dir := filepath.Dir(path)

	scanner := bufio.NewScanner(buffer.Reader())
	for scanner.Scan() {
		byteline := scanner.Bytes()

		if len(byteline) == 0 {
			continue
		}

		if !isVariant {
			after, found := bytes.CutPrefix(byteline, []byte("#EXT-X-STREAM-INF:"))
			if !found {
				continue
			}

			isVariant = true

			kvs := parseKeyValueBytes(after)

			bandwidth, _ := strconv.ParseUint(kvs["BANDWIDTH"], 10, 64)

			vari = variant{
				bandwidth:  bandwidth,
				resolution: kvs["RESOLUTION"],
				codecs:     kvs["CODECS"],
			}
		} else {
			before, _, _ := bytes.Cut(byteline, []byte("?"))
			vari.file = filepath.Join(dir, string(before))
			variants = append(variants, vari)
			isVariant = false
		}
	}

	return variants
}

// parseKeyValueBytes parses a comma-separated byte array of key=value pairs into a map[string]string.
// Quoted values (e.g. CODECS="avc1.42c01f,mp4a.40.2") preserve internal commas and are returned unquoted.
func parseKeyValueBytes(b []byte) map[string]string {
	result := make(map[string]string)
	inQuotes := false
	start := 0

	for i := 0; i < len(b); i++ {
		switch b[i] {
		case '"':
			inQuotes = !inQuotes
		case ',':
			if !inQuotes {
				parsePair(b[start:i], result)
				start = i + 1
			}
		}
	}

	if start < len(b) {
		parsePair(b[start:], result)
	}

	return result
}

func parsePair(pair []byte, result map[string]string) {
	s := strings.TrimSpace(string(pair))
	if s == "" {
		return
	}

	key, value, found := strings.Cut(s, "=")
	if !found {
		return
	}

	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)

	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		value = value[1 : len(value)-1]
	}

	result[key] = value
}

func (g *sessionRewriter) rewriteHLS(sessionID string, requestURL *url.URL, buffer *mem.Buffer) string {
	isMaster := false
	hasSession := len(sessionID) != 0

	if !hasSession {
		sessionID = shortuuid.New()
	}

	// Find all URLS in the .m3u8 and add the session ID to the query string
	scanner := bufio.NewScanner(g.buffer.Reader())
	for scanner.Scan() {
		byteline := scanner.Bytes()

		// Write empty lines unmodified
		if len(byteline) == 0 {
			buffer.Write(byteline)
			buffer.WriteByte('\n')
			continue
		}

		// Write comments unmodified
		if byteline[0] == '#' {
			buffer.Write(byteline)
			buffer.WriteByte('\n')
			continue
		}

		u, err := url.Parse(string(byteline))
		if err != nil {
			buffer.Write(byteline)
			buffer.WriteByte('\n')
			continue
		}

		// Write anything that doesn't end in .m3u8 or .ts unmodified
		if !strings.HasSuffix(u.Path, ".m3u8") && !strings.HasSuffix(u.Path, ".ts") && !strings.HasSuffix(u.Path, ".mp4") {
			buffer.Write(byteline)
			buffer.WriteByte('\n')
			continue
		}

		q := url.Values{}

		for key, values := range requestURL.Query() {
			for _, value := range values {
				q.Add(key, value)
			}
		}

		for key, values := range u.Query() {
			for _, value := range values {
				q.Set(key, value)
			}
		}

		loop := false

		// If this is a master manifest (i.e. an m3u8 which contains references to other m3u8), then
		// we give each substream an own session ID if they don't have already.
		if strings.HasSuffix(u.Path, ".m3u8") {
			// Check if we're referring to ourselves. This will cause an infinite loop
			// and has to be stopped.
			file := u.Path
			if !strings.HasPrefix(file, "/") {
				dir := urlpath.Dir(requestURL.Path)
				file = filepath.Join(dir, file)
			}

			if requestURL.Path == file {
				loop = true
			}

			q.Set("session", sessionID)

			isMaster = true
		} else {
			q.Set("session", sessionID)
		}

		u.RawQuery = q.Encode()

		if loop {
			buffer.WriteString("# m3u8 is referencing itself: " + u.String() + "\n")
		} else {
			buffer.WriteString(u.String() + "\n")
		}
	}

	if err := scanner.Err(); err != nil {
		return sessionID
	}

	// If this is not a master manifest and there isn't a session ID, we add a new session ID.
	if !isMaster && !hasSession {
		buffer.Reset()

		buffer.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-STREAM-INF:BANDWIDTH=1024\n")

		// Add the session ID to the query string
		q := requestURL.Query()
		q.Set("session", sessionID)

		buffer.WriteString(urlpath.Base(requestURL.Path) + "?" + q.Encode())
	}

	return sessionID
}
