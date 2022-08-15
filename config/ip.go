package config

import (
	"io"
	"net/http"
	"sync"
	"time"
)

// SetPublicIPs will try to figure out the public IPs (v4 and v6)
// we're running on. There's a timeout of max. 5 seconds to do it.
// If it fails, the IPs will simply not be set.
func (d *Config) SetPublicIPs() {
	var wg sync.WaitGroup

	ipv4 := ""
	ipv6 := ""

	wg.Add(2)

	go func() {
		defer wg.Done()

		ipv4 = doRequest("https://api.ipify.org")
	}()

	go func() {
		defer wg.Done()

		ipv6 = doRequest("https://api6.ipify.org")
	}()

	wg.Wait()

	if len(ipv4) != 0 {
		d.Host.Name = append(d.Host.Name, ipv4)
	}

	if len(ipv6) != 0 && ipv4 != ipv6 {
		d.Host.Name = append(d.Host.Name, ipv6)
	}
}

func doRequest(url string) string {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return ""
	}

	resp, err := client.Do(req)
	if err != nil {
		return ""
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	if resp.StatusCode != 200 {
		return ""
	}

	return string(body)
}
