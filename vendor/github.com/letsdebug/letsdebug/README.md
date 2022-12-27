# Let's Debug

[![Build Status](https://travis-ci.org/letsdebug/letsdebug.svg?branch=master)](https://travis-ci.org/letsdebug/letsdebug)
[![godoc](https://godoc.org/github.com/letsdebug/letsdebug?status.svg)](https://godoc.org/github.com/letsdebug/letsdebug)

Let's Debug is a diagnostic website, API, CLI and Go package for quickly and accurately finding and reporting issues for any domain that may prevent issuance of a Let's Encrypt SSL certificate for any ACME validation method.

It is motivated by [this community thread](https://community.letsencrypt.org/t/creating-a-webservice-for-analysis-of-common-problems/45836).

## Status
Currently [deployed to letsdebug.net and regularly in use](https://letsdebug.net).

## Problems Detected

| Name | Description | Examples
-------|-------------|--------|
| InvalidMethod, ValidationMethodDisabled, ValidationMethodNotSuitable | Checks the ACME validation method is valid and usable for the provided domain name. | [Example](https://letsdebug.net/*.letsencrypt.org/1) |
| InvalidDomain | Checks the domain is a valid domain name on a public TLD. | [Example](https://letsdebug.net/ooga.booga/2) |
| StatusNotOperational| Checks that the Let's Encrypt service is not experiencing an outage, according to status.io | - 
| DNSLookupFailed, TXTRecordError | Checks that the Unbound resolver (via libunbound) is able to resolve a variety records relevant to Let's Encrypt. Discovers problems such as DNSSEC issues, 0x20 mixed case randomization, timeouts etc, in the spirit of jsha's unboundtest.com | [Example](https://letsdebug.net/dnssec-failed.org/3) |
CAAIssuanceNotAllowed | Checks that no CAA records are preventing the issuance of Let's Encrypt certificates. | [Example](https://letsdebug.net/id-rsa.pub/4) |
CAACriticalUnknown | Checks that no CAA critical flags unknown to Let's Encrypt are used | - |
RateLimit | Checks that the domain name is not currently affected by any of the domain-based rate limits imposed by Let's Encrypt, using the public certwatch Postgres interface from Comodo's crt.sh. | [Example](https://letsdebug.net/targettec.ddns.net/13) |
NoRecords, ReservedAddress | Checks that sufficient valid A/AAAA records are present to perform HTTP-01 validation | [Example](https://letsdebug.net/localtest.me/6) |
BadRedirect | Checks that no bad HTTP redirects are present. Discovers redirects that aren't accessible, unacceptable ports, unacceptable schemes, accidental missing trailing slash on redirect. | [Example](https://letsdebug.net/foo.monkas.xyz/7) |
WebserverMisconfiguration | Checks whether the server is serving the wrong protocol on the wrong port as the result of an HTTP-01 validation request. | - |
ANotWorking, AAAANotWorking | Checks whether listed IP addresses are not functioning properly for HTTP-01 validation, including timeouts and other classes of network and HTTP errors. | [Example](https://letsdebug.net/network-fail.foo.monkas.xyz/8) |
MultipleIPAddressDiscrepancy | For domains with multiple A/AAAA records, checks whether there are major discrepancies between the server responses to reveal when the addresses may be pointing to different servers accidentally. | [Example](https://letsdebug.net/v4v6fail.monkas.xyz/51916)
CloudflareCDN | Checks whether the domain is being served via Cloudflare's proxy service (and therefore SSL termination is occurring at Cloudflare) | - |
CloudflareSSLNotProvisioned | Checks whether the domain has its SSL terminated by Cloudflare and Cloudflare has not provisioned a certificate yet (leading to a TLS handshake error). | [Example](https://letsdebug.net/cf-no-ssl.fleetssl.com/10) |
IssueFromLetsEncrypt | Attempts to detect issues with a high degree of accuracy via the Let's Encrypt v2 staging service by attempting to perform an authorization for the domain. Discovers issues such as CA-based domain blacklists & other policies, specific networking issues. | [Example](https://letsdebug.net/bankofamerica.com/12) |
| TXTDoubleLabel | Checks for the presence of records that are doubled up (e.g. `_acme-challenge.example.org.example.org`). Usually indicates that the user has been incorrectly creating records in their DNS user interface. | [Example](https://letsdebug.net/double.monkas.xyz/2477) |
PortForwarding | Checks whether the domain is serving a modem-router administrative interface instead of an intended webserver, which is indicative of a port-forwarding misconfiguration. | [Example](https://letsdebug.net/cdkauffmannnextcloud.duckdns.org/11450) |
| SanctionedDomain | Checks whether the Registered Domain is present on the [USG OFAC SDN List](https://sanctionssearch.ofac.treas.gov/). Updated daily. | [Example](https://letsdebug.net/unomasuno.com.mx/48081) |
| BlockedByNginxTestCookie | Checks whether the HTTP-01 validation requests are being intercepted by [testcookie-nginx-module](https://github.com/kyprizel/testcookie-nginx-module). | [Example](https://letsdebug.net/13513427185.ifastnet.org/51860) |
| HttpOnHttpsPort | Checks whether the server reported receiving an HTTP request on an HTTPS-only port | [Example](https://letsdebug.net/clep-energy.org/107591) |

## Web API Usage

There is a JSON-based API available as part of the web frontend.

### Submitting a test

```bash
$ curl --data '{"method":"http-01","domain":"letsdebug.net"}' -H 'content-type: application/json' https://letsdebug.net
```
```javascript
{"Domain":"letsdebug.net","ID":14}
```

### Submitting a test with custom options

```bash
curl --data '{"method":"http-01","domain":"letsdebug.net","options":{"http_request_path":"custom-path","http_expect_response":"abc123"}}' -H 'content-type: application/json' https://letsdebug.net
```

Available options are as follows:

| Option | Description |
-------|-------------|
`http_request_path` | What path within `/.well-known/acme-challenge/` to use instead of `letsdebug-test` (default) for the HTTP check. Max length 255. |
`http_expect_response` | What exact response to expect from each server during the HTTP check. By default, no particular response is expected. If present and the response does not match, the test will fail with an Error severity. It is highly recommended to always use a completely random value. Max length 255. |

### Viewing tests

```bash
$ curl -H 'accept: application/json' https://letsdebug.net/letsdebug.net/14
```
```javascript
{"id":14,"domain":"letsdebug.net","method":"http-01","status":"Complete","created_at":"2018-04-30T01:58:34.765829Z","started_at":"2018-04-30T01:58:34.769815Z","completed_at":"2018-04-30T01:58:41.39023Z","result":{}}
```

or to view all recent tests

```bash
$ curl -H 'accept: application/json' https://letsdebug.net/letsdebug.net
```

### Performing a query against the Certwatch database

```bash
$ curl "https://letsdebug.net/certwatch-query?q=<urlencoded SQL query>"
```
```javascript
{
  "query": "select c.id as crtsh_id, x509_subjectName(c.CERTIFICATE), x509_notAfter(c.CERTIFICATE) from certificate c where x509_notAfter(c.CERTIFICATE) = '2018-06-01 16:25:44' AND x509_issuerName(c.CERTIFICATE) LIKE 'C=US, O=Let''s Encrypt%';",
  "results": [
    {
      "crtsh_id": 346300797,
      "x509_notafter": "2018-06-01T16:25:44Z",
      "x509_subjectname": "CN=hivdatingzimbabwe.com"
    },
    /* ... */
  ]
}
```

## CLI Usage

You can download binaries for tagged releases for Linux for both the CLi and the server [from the releases page](https://github.com/letsdebug/letsdebug/releases). 


    letsdebug-cli -domain example.org -method http-01 -debug

## Library Usage

```go

import "github.com/letsdebug/letsdebug"

problems, _ := letsdebug.Check("example.org", letsdebug.HTTP01)
```

## Installation

### Dependencies

This package relies on a fairly recent version of libunbound.

* On Debian-based distributions:

    `apt install libunbound2 libunbound-dev`

* On EL-based distributions, you may need to build from source because the packages are ancient on e.g. CentOS, but you can try:

    `yum install unbound-libs unbound-devel`

* On OSX, [Homebrew](https://brew.sh/) contains the latest version of unbound:

    `brew install unbound`

You will also need Go's [dep](https://github.com/golang/dep) dependency manager.

### Releases
You can save time by [downloading tagged releases for 64-bit Linux](https://github.com/letsdebug/letsdebug/releases). Keep in mind you will still need to have libunbound present on your system.

### Building

    go get -u github.com/letsdebug/letsdebug/...
    cd $GOPATH/src/github.com/letsdebug/letsdebug
    make deps
    make letsdebug-cli letsdebug-server


## Contributing
Any contributions containing JavaScript will be discarded, but other feedback, bug reports, suggestions and enhancements are welcome - please open an issue first.

## LICENSE

    MIT License

    Copyright (c) 2018 Let's Debug

    Permission is hereby granted, free of charge, to any person obtaining a copy
    of this software and associated documentation files (the "Software"), to deal
    in the Software without restriction, including without limitation the rights
    to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
    copies of the Software, and to permit persons to whom the Software is
    furnished to do so, subject to the following conditions:

    The above copyright notice and this permission notice shall be included in all
    copies or substantial portions of the Software.

    THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
    IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
    FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
    AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
    LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
    OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
    SOFTWARE.