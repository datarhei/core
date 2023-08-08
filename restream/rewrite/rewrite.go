// Package rewrite provides facilities for rewriting a local HLS, RTMP, and SRT address.
package rewrite

import (
	"fmt"
	"net/url"

	"github.com/datarhei/core/v16/iam"
	iamidentity "github.com/datarhei/core/v16/iam/identity"
	"github.com/datarhei/core/v16/rtmp"
	srturl "github.com/datarhei/core/v16/srt/url"
)

type Access string

var (
	READ  Access = "read"
	WRITE Access = "write"
)

type Config struct {
	HTTPBase string
	RTMPBase string
	SRTBase  string
	IAM      iam.IAM
}

// to a new identity, i.e. adjusting the credentials to the given identity.
type Rewriter interface {
	RewriteAddress(address, user string, mode Access) string
}

type rewrite struct {
	httpBase string
	rtmpBase string
	srtBase  string
	iam      iam.IAM
}

func New(config Config) (Rewriter, error) {
	r := &rewrite{
		httpBase: config.HTTPBase,
		rtmpBase: config.RTMPBase,
		srtBase:  config.SRTBase,
		iam:      config.IAM,
	}

	if r.iam == nil {
		return nil, fmt.Errorf("missing IAM")
	}

	return r, nil
}

func (g *rewrite) RewriteAddress(address, user string, mode Access) string {
	u, err := url.Parse(address)
	if err != nil {
		return address
	}

	// Decide whether this is our local server
	if !g.isLocal(u) {
		return address
	}

	identity, _ := g.iam.GetVerifier(user)

	if identity == nil {
		return address
	}

	if u.Scheme == "http" || u.Scheme == "https" {
		return g.httpURL(u, mode, identity)
	} else if u.Scheme == "rtmp" {
		return g.rtmpURL(u, mode, identity)
	} else if u.Scheme == "srt" {
		return g.srtURL(u, mode, identity)
	}

	return address
}

func (g *rewrite) isLocal(u *url.URL) bool {
	var base *url.URL
	var err error

	if u.Scheme == "http" || u.Scheme == "https" {
		base, err = url.Parse(g.httpBase)
	} else if u.Scheme == "rtmp" {
		base, err = url.Parse(g.rtmpBase)
	} else if u.Scheme == "srt" {
		base, err = url.Parse(g.srtBase)
	} else {
		err = fmt.Errorf("unsupported scheme")
	}

	if err != nil {
		return false
	}

	hostname := u.Hostname()
	port := u.Port()

	if base.Hostname() == "localhost" {
		if hostname != "localhost" && hostname != "127.0.0.1" && hostname != "::1" {
			return false
		}

		hostname = "localhost"
	}

	host := hostname + ":" + port

	return host == base.Host
}

func (g *rewrite) httpURL(u *url.URL, mode Access, identity iamidentity.Verifier) string {
	u.User = identity.GetServiceBasicAuth()

	return u.String()
}

func (g *rewrite) rtmpURL(u *url.URL, mode Access, identity iamidentity.Verifier) string {
	token := identity.GetServiceToken()

	// Remove the existing token from the path
	path, _ := rtmp.GetToken(u)
	u.Path = path

	q := u.Query()
	q.Set("token", token)

	u.RawQuery = q.Encode()

	return u.String()
}

func (g *rewrite) srtURL(u *url.URL, mode Access, identity iamidentity.Verifier) string {
	token := identity.GetServiceToken()

	q := u.Query()

	streamInfo, err := srturl.ParseStreamId(q.Get("streamid"))
	if err != nil {
		return u.String()
	}

	streamInfo.Token = token

	if mode == WRITE {
		streamInfo.Mode = "publish"
	} else {
		streamInfo.Mode = "request"
	}

	q.Set("streamid", streamInfo.String())
	u.RawQuery = q.Encode()

	return u.String()
}
