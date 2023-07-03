package value

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

// cluster peer (id@host:port)

type ClusterPeer string

func NewClusterPeer(p *string, val string) *ClusterPeer {
	*p = val

	return (*ClusterPeer)(p)
}

func (s *ClusterPeer) Set(val string) error {
	id, address, found := strings.Cut(val, "@")
	if !found || len(id) == 0 {
		return fmt.Errorf("id is missing")
	}

	// Check if the new value is only a port number
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}

	if len(host) == 0 || len(port) == 0 {
		return fmt.Errorf("invalid address: host and port must be provided")
	}

	re := regexp.MustCompile("^[0-9]+$")
	if !re.MatchString(port) {
		return fmt.Errorf("the port must be numerical")
	}

	*s = ClusterPeer(val)

	return nil
}

func (s *ClusterPeer) String() string {
	return string(*s)
}

func (s *ClusterPeer) Validate() error {
	_, address, found := strings.Cut(string(*s), "@")
	if !found {
		return fmt.Errorf("id is missing")
	}

	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}

	if len(host) == 0 || len(port) == 0 {
		return fmt.Errorf("invalid address: host and port must be provided")
	}

	re := regexp.MustCompile("^[0-9]+$")
	if !re.MatchString(port) {
		return fmt.Errorf("the port must be numerical")
	}

	return nil
}

func (s *ClusterPeer) IsEmpty() bool {
	return s.Validate() != nil
}

// array of cluster peers

type ClusterPeerList struct {
	p         *[]string
	separator string
}

func NewClusterPeerList(p *[]string, val []string, separator string) *ClusterPeerList {
	v := &ClusterPeerList{
		p:         p,
		separator: separator,
	}

	*p = val

	return v
}

func (s *ClusterPeerList) Set(val string) error {
	list := []string{}

	for _, elm := range strings.Split(val, s.separator) {
		elm = strings.TrimSpace(elm)
		if len(elm) != 0 {
			p := NewClusterPeer(&elm, elm)
			if err := p.Validate(); err != nil {
				return fmt.Errorf("invalid cluster peer: %s: %w", elm, err)
			}
			list = append(list, elm)
		}
	}

	*s.p = list

	return nil
}

func (s *ClusterPeerList) String() string {
	if s.IsEmpty() {
		return "(empty)"
	}

	return strings.Join(*s.p, s.separator)
}

func (s *ClusterPeerList) Validate() error {
	for _, elm := range *s.p {
		elm = strings.TrimSpace(elm)
		if len(elm) != 0 {
			p := NewClusterPeer(&elm, elm)
			if err := p.Validate(); err != nil {
				return fmt.Errorf("invalid cluster peer: %s: %w", elm, err)
			}
		}
	}

	return nil
}

func (s *ClusterPeerList) IsEmpty() bool {
	return len(*s.p) == 0
}
