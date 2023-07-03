package value

import (
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/datarhei/core/v16/http/cors"
)

// optional address

type Address string

func NewAddress(p *string, val string) *Address {
	*p = val

	return (*Address)(p)
}

func (s *Address) Set(val string) error {
	if len(val) == 0 {
		*s = Address(val)
		return nil
	}

	// Check if the new value is only a port number
	re := regexp.MustCompile("^[0-9]+$")
	if re.MatchString(val) {
		val = ":" + val
	}

	*s = Address(val)
	return nil
}

func (s *Address) String() string {
	return string(*s)
}

func (s *Address) Validate() error {
	if len(string(*s)) == 0 {
		return nil
	}

	_, port, err := net.SplitHostPort(string(*s))
	if err != nil {
		return err
	}

	re := regexp.MustCompile("^[0-9]+$")
	if !re.MatchString(port) {
		return fmt.Errorf("the port must be numerical")
	}

	return nil
}

func (s *Address) IsEmpty() bool {
	return s.Validate() != nil
}

// address (host?:port)

type MustAddress string

func NewMustAddress(p *string, val string) *MustAddress {
	*p = val

	return (*MustAddress)(p)
}

func (s *MustAddress) Set(val string) error {
	// Check if the new value is only a port number
	re := regexp.MustCompile("^[0-9]+$")
	if re.MatchString(val) {
		val = ":" + val
	}

	*s = MustAddress(val)
	return nil
}

func (s *MustAddress) String() string {
	return string(*s)
}

func (s *MustAddress) Validate() error {
	_, port, err := net.SplitHostPort(string(*s))
	if err != nil {
		return err
	}

	re := regexp.MustCompile("^[0-9]+$")
	if !re.MatchString(port) {
		return fmt.Errorf("the port must be numerical")
	}

	return nil
}

func (s *MustAddress) IsEmpty() bool {
	return s.Validate() != nil
}

// full address (host:port)

type FullAddress string

func NewFullAddress(p *string, val string) *FullAddress {
	*p = val

	return (*FullAddress)(p)
}

func (s *FullAddress) Set(val string) error {
	// Check if the new value is only a port number
	host, port, err := net.SplitHostPort(val)
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

	*s = FullAddress(val)

	return nil
}

func (s *FullAddress) String() string {
	return string(*s)
}

func (s *FullAddress) Validate() error {
	host, port, err := net.SplitHostPort(string(*s))
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

func (s *FullAddress) IsEmpty() bool {
	return s.Validate() != nil
}

// array of CIDR notation IP adresses

type CIDRList struct {
	p         *[]string
	separator string
}

func NewCIDRList(p *[]string, val []string, separator string) *CIDRList {
	v := &CIDRList{
		p:         p,
		separator: separator,
	}

	*p = val

	return v
}

func (s *CIDRList) Set(val string) error {
	list := []string{}

	for _, elm := range strings.Split(val, s.separator) {
		elm = strings.TrimSpace(elm)
		if len(elm) != 0 {
			list = append(list, elm)
		}
	}

	*s.p = list

	return nil
}

func (s *CIDRList) String() string {
	if s.IsEmpty() {
		return "(empty)"
	}

	return strings.Join(*s.p, s.separator)
}

func (s *CIDRList) Validate() error {
	for _, cidr := range *s.p {
		_, _, err := net.ParseCIDR(cidr)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *CIDRList) IsEmpty() bool {
	return len(*s.p) == 0
}

// array of origins for CORS

type CORSOrigins struct {
	p         *[]string
	separator string
}

func NewCORSOrigins(p *[]string, val []string, separator string) *CORSOrigins {
	v := &CORSOrigins{
		p:         p,
		separator: separator,
	}

	*p = val

	return v
}

func (s *CORSOrigins) Set(val string) error {
	list := []string{}

	for _, elm := range strings.Split(val, s.separator) {
		elm = strings.TrimSpace(elm)
		if len(elm) != 0 {
			list = append(list, elm)
		}
	}

	*s.p = list

	return nil
}

func (s *CORSOrigins) String() string {
	if s.IsEmpty() {
		return "(empty)"
	}

	return strings.Join(*s.p, s.separator)
}

func (s *CORSOrigins) Validate() error {
	return cors.Validate(*s.p)
}

func (s *CORSOrigins) IsEmpty() bool {
	return len(*s.p) == 0
}

// network port

type Port int

func NewPort(p *int, val int) *Port {
	*p = val

	return (*Port)(p)
}

func (i *Port) Set(val string) error {
	v, err := strconv.Atoi(val)
	if err != nil {
		return err
	}
	*i = Port(v)
	return nil
}

func (i *Port) String() string {
	return strconv.Itoa(int(*i))
}

func (i *Port) Validate() error {
	val := int(*i)

	if val < 0 || val >= (1<<16) {
		return fmt.Errorf("%d is not in the range of [0, %d]", val, 1<<16-1)
	}

	return nil
}

func (i *Port) IsEmpty() bool {
	return int(*i) == 0
}

// url

type URL string

func NewURL(p *string, val string) *URL {
	*p = val

	return (*URL)(p)
}

func (u *URL) Set(val string) error {
	*u = URL(val)
	return nil
}

func (u *URL) String() string {
	return string(*u)
}

func (u *URL) Validate() error {
	val := string(*u)

	if len(val) == 0 {
		return nil
	}

	URL, err := url.Parse(val)
	if err != nil {
		return fmt.Errorf("%s is not a valid URL", val)
	}

	if len(URL.Scheme) == 0 || len(URL.Host) == 0 {
		return fmt.Errorf("%s is not a valid URL", val)
	}

	return nil
}

func (u *URL) IsEmpty() bool {
	return len(string(*u)) == 0
}

// email address

type Email string

func NewEmail(p *string, val string) *Email {
	*p = val

	return (*Email)(p)
}

func (s *Email) Set(val string) error {
	addr, err := mail.ParseAddress(val)
	if err != nil {
		return err
	}

	*s = Email(addr.Address)
	return nil
}

func (s *Email) String() string {
	return string(*s)
}

func (s *Email) Validate() error {
	if len(s.String()) == 0 {
		return nil
	}

	_, err := mail.ParseAddress(s.String())
	return err
}

func (s *Email) IsEmpty() bool {
	return len(string(*s)) == 0
}
