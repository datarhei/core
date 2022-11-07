# Core

### Core v16.10.1 > v16.11.0

-   Upgrade to FFmpeg 5.1.2
-   Add FFmpeg 4.4 to FFmpeg 5.1 migration tool
-   Add alternative SRT streamid
-   Fix crash with custom SSL certificates (datarhei/restreamer#425)
-   Fix proper version handling for config
-   Fix widged session data
-   Fix resetting process stats when process stopped
-   Fix stale FFmpeg process detection for streams with only audio
-   Fix wrong return status code (#6)
-   Fix use SRT defaults for key material exchange

### Core v16.10.0 > v16.10.1

-   Add email address in TLS config for Let's Encrypt
-   Fix use of Let's Encrypt production CA

### Core v16.9.1 > v16.10.0

-   Add HLS session middleware to diskfs
-   Add /v3/metrics (get) endpoint to list all known metrics
-   Add logging HTTP request and response body sizes
-   Add process id and reference glob pattern matching
-   Add cache block list for extensions not to cache
-   Mod exclude .m3u8 and .mpd files from disk cache by default
-   Mod replaces x/crypto/acme/autocert with caddyserver/certmagic
-   Mod exposes ports (Docker desktop)
-   Fix assigning cleanup rules for diskfs
-   Fix wrong path for swagger definition
-   Fix process cleanup on delete, remove empty directories from disk
-   Fix SRT blocking port on restart (upgrade datarhei/gosrt)
-   Fix RTMP communication (Blackmagic Web Presenter, thx 235 MEDIA)
-   Fix RTMP communication (Blackmagic ATEM Mini, datarhei/restreamer#385)
-   Fix injecting commit, branch, and build info
-   Fix API metadata endpoints responses

#### Core v16.9.0 > v16.9.1

-   Fix v1 import app
-   Fix race condition

#### Core v16.8.0 > v16.9.0

-   Add new placeholders and parameters for placeholder
-   Allow RTMP server if RTMPS server is enabled. In case you already had RTMPS enabled it will listen on the same port as before. An RTMP server will be started additionally listening on a lower port number. The RTMP app is required to start with a slash.
-   Add optional escape character to process placeholder
-   Fix output address validation for tee outputs
-   Fix updating process config
-   Add experimental SRT connection stats and logs API
-   Hide /config/reload endpoint in reade-only mode
-   Add experimental SRT server (datarhei/gosrt)
-   Create v16 in go.mod
-   Fix data races, tests, lint, and update dependencies
-   Add trailing slash for routed directories (datarhei/restreamer#340)
-   Allow relative URLs in content in static routes

#### Core v16.7.2 > v16.8.0

-   Add purge_on_delete function
-   Mod updated dependencies
-   Mod updated API docs
-   Fix disabled session logging
-   Fix FFmpeg skills reload
-   Fix ignores processes with invalid references (thx Patron Ramakrishna Chillara)
-   Fix code scanning alerts
