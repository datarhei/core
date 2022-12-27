package letsdebug

import (
	"fmt"
	"net"
	"strings"

	"github.com/miekg/dns"
	"github.com/miekg/unbound"
)

var (
	reservedNets []*net.IPNet
)

func lookup(name string, rrType uint16) ([]dns.RR, error) {
	ub := unbound.New()
	defer ub.Destroy()

	if err := setUnboundConfig(ub); err != nil {
		return nil, fmt.Errorf("Failed to configure Unbound resolver: %v", err)
	}

	result, err := ub.Resolve(name, rrType, dns.ClassINET)
	if err != nil {
		return nil, err
	}

	if result.Bogus {
		return nil, fmt.Errorf("DNS response for %s had fatal DNSSEC issues: %v", name, result.WhyBogus)
	}

	if result.Rcode == dns.RcodeServerFailure || result.Rcode == dns.RcodeRefused {
		return nil, fmt.Errorf("DNS response for %s/%s did not have an acceptable response code: %s",
			name, dns.TypeToString[rrType], dns.RcodeToString[result.Rcode])
	}

	return result.Rr, nil
}

func normalizeFqdn(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimSuffix(name, ".")
	return strings.ToLower(name)
}

func isAddressReserved(ip net.IP) bool {
	for _, reserved := range reservedNets {
		if reserved.Contains(ip) {
			return true
		}
	}
	return false
}

func init() {
	reservedNets = []*net.IPNet{}
	reservedCIDRs := []string{
		"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10",
		"127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12",
		"192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24",
		"192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24",
		"203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
		"255.255.255.255/32", "::/128", "::1/128", /*"::ffff:0:0/96",*/
		"64:ff9b::/96", "100::/64", "2001::/32", "2001:10::/28",
		"2001:20::/28", "2001:db8::/32", "2002::/16", "fc00::/7",
		"fe80::/10", "ff00::/8",
	}
	for _, cidr := range reservedCIDRs {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(err)
		}
		reservedNets = append(reservedNets, n)
	}
}

func setUnboundConfig(ub *unbound.Unbound) error {
	// options need the : in the option key according to docs
	opts := []struct {
		Opt string
		Val string
	}{
		{"verbosity:", "0"},
		{"use-syslog:", "no"},
		{"do-ip4:", "yes"},
		{"do-ip6:", "yes"},
		{"do-udp:", "yes"},
		{"do-tcp:", "yes"},
		{"tcp-upstream:", "no"},
		{"harden-glue:", "yes"},
		{"harden-dnssec-stripped:", "yes"},
		{"cache-min-ttl:", "0"},
		{"cache-max-ttl:", "0"},
		{"cache-max-negative-ttl:", "0"},
		{"neg-cache-size:", "0"},
		{"prefetch:", "no"},
		{"unwanted-reply-threshold:", "10000"},
		{"do-not-query-localhost:", "yes"},
		{"val-clean-additional:", "yes"},
		{"harden-algo-downgrade:", "yes"},
		{"edns-buffer-size:", "512"},
		{"val-sig-skew-min:", "0"},
		{"val-sig-skew-max:", "0"},
		{"target-fetch-policy:", "0 0 0 0 0"},
	}

	for _, opt := range opts {
		// Can't ignore these because we cant silently have policies being ignored
		if err := ub.SetOption(opt.Opt, opt.Val); err != nil {
			return fmt.Errorf("Failed to configure unbound with option %s %v", opt.Opt, err)
		}
	}

	// use-caps-for-id was bugged (no colon) < 1.7.1, try both ways in order to be compatible
	// https://www.nlnetlabs.nl/bugs-script/show_bug.cgi?id=4092
	if err := ub.SetOption("use-caps-for-id:", "yes"); err != nil {
		if err = ub.SetOption("use-caps-for-id", "yes"); err != nil {
			return fmt.Errorf("Failed to configure unbound with use-caps-for-id: %v", err)
		}
	}

	return ub.AddTa(`.       172800  IN      DNSKEY  257 3 8 AwEAAaz/tAm8yTn4Mfeh5eyI96WSVexTBAvkMgJzkKTOiW1vkIbzxeF3+/4RgWOq7HrxRixHlFlExOLAJr5emLvN7SWXgnLh4+B5xQlNVz8Og8kvArMtNROxVQuCaSnIDdD5LKyWbRd2n9WGe2R8PzgCmr3EgVLrjyBxWezF0jLHwVN8efS3rCj/EWgvIWgb9tarpVUDK/b58Da+sqqls3eNbuv7pr+eoZG+SrDK6nWeL3c6H5Apxz7LjVc1uTIdsIXxuOLYA4/ilBmSVIzuDWfdRUfhHdY6+cn8HFRm+2hM8AnXGXws9555KrUB5qihylGa8subX2Nn6UwNR1AkUTV74bU=
	.       172800  IN      DNSKEY  256 3 8 AwEAAdp440E6Mz7c+Vl4sPd0lTv2Qnc85dTW64j0RDD7sS/zwxWDJ3QRES2VKDO0OXLMqVJSs2YCCSDKuZXpDPuf++YfAu0j7lzYYdWTGwyNZhEaXtMQJIKYB96pW6cRkiG2Dn8S2vvo/PxW9PKQsyLbtd8PcwWglHgReBVp7kEv/Dd+3b3YMukt4jnWgDUddAySg558Zld+c9eGWkgWoOiuhg4rQRkFstMX1pRyOSHcZuH38o1WcsT4y3eT0U/SR6TOSLIB/8Ftirux/h297oS7tCcwSPt0wwry5OFNTlfMo8v7WGurogfk8hPipf7TTKHIi20LWen5RCsvYsQBkYGpF78=
	.       172800  IN      DNSKEY  257 3 8 AwEAAagAIKlVZrpC6Ia7gEzahOR+9W29euxhJhVVLOyQbSEW0O8gcCjFFVQUTf6v58fLjwBd0YI0EzrAcQqBGCzh/RStIoO8g0NfnfL2MTJRkxoXbfDaUeVPQuYEhg37NZWAJQ9VnMVDxP/VHL496M/QZxkjf5/Efucp2gaDX6RS6CXpoY68LsvPVjR0ZSwzz1apAzvN9dlzEheX7ICJBBtuA6G3LQpzW5hOA2hzCTMjJPJ8LbqF6dsV6DoBQzgul0sGIcGOYl7OyQdXfZ57relSQageu+ipAdTTJ25AsRTAoub8ONGcLmqrAmRLKBP1dfwhYB4N7knNnulqQxA+Uk1ihz0=
	.       172800  IN      RRSIG   DNSKEY 8 0 172800 20181101000000 20181011000000 20326 . M/LTswhCjuJUTvX1CFqC+TiJ4Fez7AROa5mM+1AI2MJ+zLHhr3JaMxyydFLWrBHR0056Hz7hNqQ9i63hGeiR6uMfanF0jIRb9XqgGP8nY37T8ESpS1UiM9rJn4b40RFqDSEvuFdd4hGwK3EX0snOCLdUT8JezxtreXI0RilmqDC2g44TAKyFw+Is9Qwl+k6+fbMQ/atA8adANbYgyuHfiwQCCUtXRaTCpRgQtsAz9izO0VYIGeHIoJta0demAIrLCOHNVH2ogHTqMEQ18VqUNzTd0aGURACBdS7PeP2KogPD7N8Q970O84TFmO4ahPIvqO+milCn5OQTbbgsjHqY6Q==`)
}
