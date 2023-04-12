package value

import (
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// array of s3 storages
// https://access_key_id:secret_access_id@region.endpoint/bucket?name=aaa&mount=/abc&username=xxx&password=yyy

type S3Storage struct {
	Name       string `json:"name"`
	Mountpoint string `json:"mountpoint"`
	Auth       struct {
		Enable   bool   `json:"enable"`
		Username string `json:"username"`
		Password string `json:"password"`
	} `json:"auth"`
	Endpoint        string `json:"endpoint"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	Bucket          string `json:"bucket"`
	Region          string `json:"region"`
	UseSSL          bool   `json:"use_ssl"`
}

func (t *S3Storage) String() string {
	u := url.URL{}

	if t.UseSSL {
		u.Scheme = "https"
	} else {
		u.Scheme = "http"
	}

	u.User = url.UserPassword(t.AccessKeyID, "---")

	u.Host = t.Endpoint

	if len(t.Region) != 0 {
		u.Host = t.Region + "." + u.Host
	}

	if len(t.Bucket) != 0 {
		u.Path = "/" + t.Bucket
	}

	v := url.Values{}
	v.Set("name", t.Name)
	v.Set("mountpoint", t.Mountpoint)

	if t.Auth.Enable {
		if len(t.Auth.Username) != 0 {
			v.Set("username", t.Auth.Username)
		}

		if len(t.Auth.Password) != 0 {
			v.Set("password", "---")
		}
	}

	u.RawQuery = v.Encode()

	return u.String()
}

type s3StorageListValue struct {
	p         *[]S3Storage
	separator string
}

func NewS3StorageListValue(p *[]S3Storage, val []S3Storage, separator string) *s3StorageListValue {
	v := &s3StorageListValue{
		p:         p,
		separator: separator,
	}

	*p = val
	return v
}

func (s *s3StorageListValue) Set(val string) error {
	list := []S3Storage{}

	for _, elm := range strings.Split(val, s.separator) {
		u, err := url.Parse(elm)
		if err != nil {
			return fmt.Errorf("invalid S3 storage URL (%s): %w", elm, err)
		}

		t := S3Storage{
			Name:        u.Query().Get("name"),
			Mountpoint:  u.Query().Get("mountpoint"),
			AccessKeyID: u.User.Username(),
		}

		hostname := u.Hostname()
		port := u.Port()

		domain, err := publicsuffix.EffectiveTLDPlusOne(hostname)
		if err != nil {
			return fmt.Errorf("invalid eTLD (%s): %w", hostname, err)
		}

		t.Endpoint = domain
		if len(port) != 0 {
			t.Endpoint += ":" + port
		}

		region := strings.TrimSuffix(hostname, domain)
		if len(region) != 0 {
			t.Region = strings.TrimSuffix(region, ".")
		}

		secret, ok := u.User.Password()
		if ok {
			t.SecretAccessKey = secret
		}

		t.Bucket = strings.TrimPrefix(u.Path, "/")

		if u.Scheme == "https" {
			t.UseSSL = true
		}

		if u.Query().Has("username") || u.Query().Has("password") {
			t.Auth.Enable = true
			t.Auth.Username = u.Query().Get("username")
			t.Auth.Username = u.Query().Get("password")
		}

		list = append(list, t)
	}

	*s.p = list

	return nil
}

func (s *s3StorageListValue) String() string {
	if s.IsEmpty() {
		return "(empty)"
	}

	list := []string{}

	for _, t := range *s.p {
		list = append(list, t.String())
	}

	return strings.Join(list, s.separator)
}

func (s *s3StorageListValue) Validate() error {
	for i, t := range *s.p {
		if len(t.Name) == 0 {
			return fmt.Errorf("the name for s3 storage %d is missing", i)
		}

		if len(t.Mountpoint) == 0 {
			return fmt.Errorf("the mountpoint for s3 storage %d is missing", i)
		}

		if t.Auth.Enable {
			if len(t.Auth.Username) == 0 && len(t.Auth.Password) == 0 {
				return fmt.Errorf("auth is enabled, but no username and password are set for s3 storage %d", i)
			}
		}
	}

	return nil
}

func (s *s3StorageListValue) IsEmpty() bool {
	return len(*s.p) == 0
}
