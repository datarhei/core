package url

import (
	"fmt"
	neturl "net/url"
	"regexp"
	"strings"
)

type URL struct {
	Scheme   string
	Host     string
	StreamId string
	Options  neturl.Values
}

func Parse(srturl string) (*URL, error) {
	u, err := neturl.Parse(srturl)
	if err != nil {
		return nil, err
	}

	if u.Scheme != "srt" {
		return nil, fmt.Errorf("invalid SRT url")
	}

	options := u.Query()
	streamid := options.Get("streamid")
	options.Del("streamid")

	su := &URL{
		Scheme:   "srt",
		Host:     u.Host,
		StreamId: streamid,
		Options:  options,
	}

	return su, nil
}

func (su *URL) String() string {
	options, _ := neturl.ParseQuery(su.Options.Encode())
	options.Set("streamid", su.StreamId)

	u := neturl.URL{
		Scheme:   su.Scheme,
		Host:     su.Host,
		RawQuery: options.Encode(),
	}

	return u.String()
}

func (su *URL) StreamInfo() (*StreamInfo, error) {
	s, err := ParseStreamId(su.StreamId)
	if err != nil {
		return nil, err
	}

	return &s, nil
}

func (su *URL) SetStreamInfo(si *StreamInfo) {
	su.StreamId = si.String()
}

func (su *URL) Hostname() string {
	u := neturl.URL{
		Host: su.Host,
	}

	return u.Hostname()
}

func (su *URL) Port() string {
	u := neturl.URL{
		Host: su.Host,
	}

	return u.Port()
}

type StreamInfo struct {
	Mode     string
	Resource string
	Token    string
}

func (si *StreamInfo) String() string {
	streamid := si.Resource

	if len(si.Mode) != 0 && si.Mode != "request" {
		streamid += ",mode:" + si.Mode
	}

	if len(si.Token) != 0 {
		streamid += ",token:" + si.Token
	}

	return streamid
}

// ParseStreamId parses a streamid. If the streamid is in the old format
// it is detected and parsed accordingly. Otherwise the new simplified
// format will be assumed.
//
// resource[,token:{token}]?[,mode:(publish|*request)]?
//
// If the mode is not provided, "request" will be assumed.
func ParseStreamId(streamid string) (StreamInfo, error) {
	si := StreamInfo{Mode: "request"}

	if decodedStreamid, err := neturl.QueryUnescape(streamid); err == nil {
		streamid = decodedStreamid
	}

	if strings.HasPrefix(streamid, "#!:") {
		return ParseDeprecatedStreamId(streamid)
	}

	re := regexp.MustCompile(`,(token|mode):(.+)`)

	results := map[string]string{}

	idEnd := -1
	value := streamid
	key := ""

	for {
		matches := re.FindStringSubmatchIndex(value)
		if matches == nil {
			break
		}

		if idEnd < 0 {
			idEnd = matches[2] - 1
		}

		if len(key) != 0 {
			results[key] = value[:matches[2]-1]
		}

		key = value[matches[2]:matches[3]]
		value = value[matches[4]:matches[5]]

		results[key] = value
	}

	if idEnd < 0 {
		idEnd = len(streamid)
	}

	si.Resource = streamid[:idEnd]
	if token, ok := results["token"]; ok {
		si.Token = token
	}

	if mode, ok := results["mode"]; ok {
		si.Mode = mode
	} else {
		si.Mode = "request"
	}

	return si, nil
}

// ParseDeprecatedStreamId parses a streamid in the old format. The old format
// is based on the recommendation of the SRT specs, but with the special
// character it contains it can cause some trouble in clients (e.g. kiloview
// doesn't like the = character).
func ParseDeprecatedStreamId(streamid string) (StreamInfo, error) {
	si := StreamInfo{Mode: "request"}

	if !strings.HasPrefix(streamid, "#!:") {
		return si, fmt.Errorf("unknown streamid format")
	}

	streamid = strings.TrimPrefix(streamid, "#!:")

	kvs := strings.Split(streamid, ",")

	split := func(s, sep string) (string, string, error) {
		splitted := strings.SplitN(s, sep, 2)

		if len(splitted) != 2 {
			return "", "", fmt.Errorf("invalid key/value pair")
		}

		return splitted[0], splitted[1], nil
	}

	for _, kv := range kvs {
		key, value, err := split(kv, "=")
		if err != nil {
			continue
		}

		switch key {
		case "m":
			si.Mode = value
		case "r":
			si.Resource = value
		case "token":
			si.Token = value
		default:
		}
	}

	return si, nil
}
