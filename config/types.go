package config

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/datarhei/core/v16/http/cors"

	"golang.org/x/net/publicsuffix"
)

type value interface {
	// String returns a string representation of the value.
	String() string

	// Set a new value for the value. Returns an
	// error if the given string representation can't
	// be transformed to the value. Returns nil
	// if the new value has been set.
	Set(string) error

	// Validate the value. The returned error will
	// indicate what is wrong with the current value.
	// Returns nil if the value is OK.
	Validate() error

	// IsEmpty returns whether the value represents an empty
	// representation for that value.
	IsEmpty() bool
}

// string

type stringValue string

func newStringValue(p *string, val string) *stringValue {
	*p = val
	return (*stringValue)(p)
}

func (s *stringValue) Set(val string) error {
	*s = stringValue(val)
	return nil
}

func (s *stringValue) String() string {
	return string(*s)
}

func (s *stringValue) Validate() error {
	return nil
}

func (s *stringValue) IsEmpty() bool {
	return len(string(*s)) == 0
}

// address (host?:port)

type addressValue string

func newAddressValue(p *string, val string) *addressValue {
	*p = val
	return (*addressValue)(p)
}

func (s *addressValue) Set(val string) error {
	// Check if the new value is only a port number
	re := regexp.MustCompile("^[0-9]+$")
	if re.MatchString(val) {
		val = ":" + val
	}

	*s = addressValue(val)
	return nil
}

func (s *addressValue) String() string {
	return string(*s)
}

func (s *addressValue) Validate() error {
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

func (s *addressValue) IsEmpty() bool {
	return s.Validate() != nil
}

// array of strings

type stringListValue struct {
	p         *[]string
	separator string
}

func newStringListValue(p *[]string, val []string, separator string) *stringListValue {
	v := &stringListValue{
		p:         p,
		separator: separator,
	}
	*p = val
	return v
}

func (s *stringListValue) Set(val string) error {
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

func (s *stringListValue) String() string {
	if s.IsEmpty() {
		return "(empty)"
	}

	return strings.Join(*s.p, s.separator)
}

func (s *stringListValue) Validate() error {
	return nil
}

func (s *stringListValue) IsEmpty() bool {
	return len(*s.p) == 0
}

// array of auth0 tenants

type tenantListValue struct {
	p         *[]Auth0Tenant
	separator string
}

func newTenantListValue(p *[]Auth0Tenant, val []Auth0Tenant, separator string) *tenantListValue {
	v := &tenantListValue{
		p:         p,
		separator: separator,
	}

	*p = val
	return v
}

func (s *tenantListValue) Set(val string) error {
	list := []Auth0Tenant{}

	for i, elm := range strings.Split(val, s.separator) {
		data, err := base64.StdEncoding.DecodeString(elm)
		if err != nil {
			return fmt.Errorf("invalid base64 encoding of tenant %d: %w", i, err)
		}

		t := Auth0Tenant{}
		if err := json.Unmarshal(data, &t); err != nil {
			return fmt.Errorf("invalid JSON in tenant %d: %w", i, err)
		}

		list = append(list, t)
	}

	*s.p = list

	return nil
}

func (s *tenantListValue) String() string {
	if s.IsEmpty() {
		return "(empty)"
	}

	list := []string{}

	for _, t := range *s.p {
		list = append(list, fmt.Sprintf("%s (%d users)", t.Domain, len(t.Users)))
	}

	return strings.Join(list, ",")
}

func (s *tenantListValue) Validate() error {
	for i, t := range *s.p {
		if len(t.Domain) == 0 {
			return fmt.Errorf("the domain for tenant %d is missing", i)
		}

		if len(t.Audience) == 0 {
			return fmt.Errorf("the audience for tenant %d is missing", i)
		}
	}

	return nil
}

func (s *tenantListValue) IsEmpty() bool {
	return len(*s.p) == 0
}

// array of s3 storages
// https://access_key_id:secret_access_id@region.endpoint/bucket?name=aaa&mount=/abc&username=xxx&password=yyy

type s3StorageListValue struct {
	p         *[]S3Storage
	separator string
}

func newS3StorageListValue(p *[]S3Storage, val []S3Storage, separator string) *s3StorageListValue {
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

		list = append(list, u.String())
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

// map of strings to strings

type stringMapStringValue struct {
	p *map[string]string
}

func newStringMapStringValue(p *map[string]string, val map[string]string) *stringMapStringValue {
	v := &stringMapStringValue{
		p: p,
	}

	if *p == nil {
		*p = make(map[string]string)
	}

	if val != nil {
		*p = val
	}

	return v
}

func (s *stringMapStringValue) Set(val string) error {
	mappings := make(map[string]string)

	for _, elm := range strings.Split(val, " ") {
		elm = strings.TrimSpace(elm)
		if len(elm) == 0 {
			continue
		}

		mapping := strings.SplitN(elm, ":", 2)

		mappings[mapping[0]] = mapping[1]
	}

	*s.p = mappings

	return nil
}

func (s *stringMapStringValue) String() string {
	if s.IsEmpty() {
		return "(empty)"
	}

	mappings := make([]string, len(*s.p))

	i := 0
	for k, v := range *s.p {
		mappings[i] = k + ":" + v
		i++
	}

	return strings.Join(mappings, " ")
}

func (s *stringMapStringValue) Validate() error {
	return nil
}

func (s *stringMapStringValue) IsEmpty() bool {
	return len(*s.p) == 0
}

// array of CIDR notation IP adresses

type cidrListValue struct {
	p         *[]string
	separator string
}

func newCIDRListValue(p *[]string, val []string, separator string) *cidrListValue {
	v := &cidrListValue{
		p:         p,
		separator: separator,
	}
	*p = val
	return v
}

func (s *cidrListValue) Set(val string) error {
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

func (s *cidrListValue) String() string {
	if s.IsEmpty() {
		return "(empty)"
	}

	return strings.Join(*s.p, s.separator)
}

func (s *cidrListValue) Validate() error {
	for _, cidr := range *s.p {
		_, _, err := net.ParseCIDR(cidr)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *cidrListValue) IsEmpty() bool {
	return len(*s.p) == 0
}

// array of origins for CORS

type corsOriginsValue struct {
	p         *[]string
	separator string
}

func newCORSOriginsValue(p *[]string, val []string, separator string) *corsOriginsValue {
	v := &corsOriginsValue{
		p:         p,
		separator: separator,
	}
	*p = val
	return v
}

func (s *corsOriginsValue) Set(val string) error {
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

func (s *corsOriginsValue) String() string {
	if s.IsEmpty() {
		return "(empty)"
	}

	return strings.Join(*s.p, s.separator)
}

func (s *corsOriginsValue) Validate() error {
	return cors.Validate(*s.p)
}

func (s *corsOriginsValue) IsEmpty() bool {
	return len(*s.p) == 0
}

// boolean

type boolValue bool

func newBoolValue(p *bool, val bool) *boolValue {
	*p = val
	return (*boolValue)(p)
}

func (b *boolValue) Set(val string) error {
	v, err := strconv.ParseBool(val)
	if err != nil {
		return err
	}
	*b = boolValue(v)
	return nil
}

func (b *boolValue) String() string {
	return strconv.FormatBool(bool(*b))
}

func (b *boolValue) Validate() error {
	return nil
}

func (b *boolValue) IsEmpty() bool {
	return !bool(*b)
}

// int

type intValue int

func newIntValue(p *int, val int) *intValue {
	*p = val
	return (*intValue)(p)
}

func (i *intValue) Set(val string) error {
	v, err := strconv.Atoi(val)
	if err != nil {
		return err
	}
	*i = intValue(v)
	return nil
}

func (i *intValue) String() string {
	return strconv.Itoa(int(*i))
}

func (i *intValue) Validate() error {
	return nil
}

func (i *intValue) IsEmpty() bool {
	return int(*i) == 0
}

// int64

type int64Value int64

func newInt64Value(p *int64, val int64) *int64Value {
	*p = val
	return (*int64Value)(p)
}

func (u *int64Value) Set(val string) error {
	v, err := strconv.ParseInt(val, 0, 64)
	if err != nil {
		return err
	}
	*u = int64Value(v)
	return nil
}

func (u *int64Value) String() string {
	return strconv.FormatInt(int64(*u), 10)
}

func (u *int64Value) Validate() error {
	return nil
}

func (u *int64Value) IsEmpty() bool {
	return int64(*u) == 0
}

// uint64

type uint64Value uint64

func newUint64Value(p *uint64, val uint64) *uint64Value {
	*p = val
	return (*uint64Value)(p)
}

func (u *uint64Value) Set(val string) error {
	v, err := strconv.ParseUint(val, 0, 64)
	if err != nil {
		return err
	}
	*u = uint64Value(v)
	return nil
}

func (u *uint64Value) String() string {
	return strconv.FormatUint(uint64(*u), 10)
}

func (u *uint64Value) Validate() error {
	return nil
}

func (u *uint64Value) IsEmpty() bool {
	return uint64(*u) == 0
}

// network port

type portValue int

func newPortValue(p *int, val int) *portValue {
	*p = val
	return (*portValue)(p)
}

func (i *portValue) Set(val string) error {
	v, err := strconv.Atoi(val)
	if err != nil {
		return err
	}
	*i = portValue(v)
	return nil
}

func (i *portValue) String() string {
	return strconv.Itoa(int(*i))
}

func (i *portValue) Validate() error {
	val := int(*i)

	if val < 0 || val >= (1<<16) {
		return fmt.Errorf("%d is not in the range of [0, %d]", val, 1<<16-1)
	}

	return nil
}

func (i *portValue) IsEmpty() bool {
	return int(*i) == 0
}

// must directory

type mustDirValue string

func newMustDirValue(p *string, val string) *mustDirValue {
	*p = val
	return (*mustDirValue)(p)
}

func (u *mustDirValue) Set(val string) error {
	*u = mustDirValue(val)
	return nil
}

func (u *mustDirValue) String() string {
	return string(*u)
}

func (u *mustDirValue) Validate() error {
	val := string(*u)

	if len(strings.TrimSpace(val)) == 0 {
		return fmt.Errorf("path name must not be empty")
	}

	finfo, err := os.Stat(val)
	if err != nil {
		return fmt.Errorf("%s does not exist", val)
	}

	if !finfo.IsDir() {
		return fmt.Errorf("%s is not a directory", val)
	}

	return nil
}

func (u *mustDirValue) IsEmpty() bool {
	return len(string(*u)) == 0
}

// directory

type dirValue string

func newDirValue(p *string, val string) *dirValue {
	*p = val
	return (*dirValue)(p)
}

func (u *dirValue) Set(val string) error {
	*u = dirValue(val)
	return nil
}

func (u *dirValue) String() string {
	return string(*u)
}

func (u *dirValue) Validate() error {
	val := string(*u)

	if len(strings.TrimSpace(val)) == 0 {
		return nil
	}

	finfo, err := os.Stat(val)
	if err != nil {
		return fmt.Errorf("%s does not exist", val)
	}

	if !finfo.IsDir() {
		return fmt.Errorf("%s is not a directory", val)
	}

	return nil
}

func (u *dirValue) IsEmpty() bool {
	return len(string(*u)) == 0
}

// executable

type execValue string

func newExecValue(p *string, val string) *execValue {
	*p = val
	return (*execValue)(p)
}

func (u *execValue) Set(val string) error {
	*u = execValue(val)
	return nil
}

func (u *execValue) String() string {
	return string(*u)
}

func (u *execValue) Validate() error {
	val := string(*u)

	_, err := exec.LookPath(val)
	if err != nil {
		return fmt.Errorf("%s not found or is not executable", val)
	}

	return nil
}

func (u *execValue) IsEmpty() bool {
	return len(string(*u)) == 0
}

// regular file

type fileValue string

func newFileValue(p *string, val string) *fileValue {
	*p = val
	return (*fileValue)(p)
}

func (u *fileValue) Set(val string) error {
	*u = fileValue(val)
	return nil
}

func (u *fileValue) String() string {
	return string(*u)
}

func (u *fileValue) Validate() error {
	val := string(*u)

	if len(val) == 0 {
		return nil
	}

	finfo, err := os.Stat(val)
	if err != nil {
		return fmt.Errorf("%s does not exist", val)
	}

	if !finfo.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", val)
	}

	return nil
}

func (u *fileValue) IsEmpty() bool {
	return len(string(*u)) == 0
}

// time

type timeValue time.Time

func newTimeValue(p *time.Time, val time.Time) *timeValue {
	*p = val
	return (*timeValue)(p)
}

func (u *timeValue) Set(val string) error {
	v, err := time.Parse(time.RFC3339, val)
	if err != nil {
		return err
	}
	*u = timeValue(v)
	return nil
}

func (u *timeValue) String() string {
	v := time.Time(*u)
	return v.Format(time.RFC3339)
}

func (u *timeValue) Validate() error {
	return nil
}

func (u *timeValue) IsEmpty() bool {
	v := time.Time(*u)
	return v.IsZero()
}

// url

type urlValue string

func newURLValue(p *string, val string) *urlValue {
	*p = val
	return (*urlValue)(p)
}

func (u *urlValue) Set(val string) error {
	*u = urlValue(val)
	return nil
}

func (u *urlValue) String() string {
	return string(*u)
}

func (u *urlValue) Validate() error {
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

func (u *urlValue) IsEmpty() bool {
	return len(string(*u)) == 0
}

// absolute path

type absolutePathValue string

func newAbsolutePathValue(p *string, val string) *absolutePathValue {
	*p = filepath.Clean(val)
	return (*absolutePathValue)(p)
}

func (s *absolutePathValue) Set(val string) error {
	*s = absolutePathValue(filepath.Clean(val))
	return nil
}

func (s *absolutePathValue) String() string {
	return string(*s)
}

func (s *absolutePathValue) Validate() error {
	path := string(*s)

	if !filepath.IsAbs(path) {
		return fmt.Errorf("%s is not an absolute path", path)
	}

	return nil
}

func (s *absolutePathValue) IsEmpty() bool {
	return len(string(*s)) == 0
}
