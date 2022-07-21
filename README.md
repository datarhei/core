# Core

[![CodeQL](https://github.com/datarhei/core/workflows/CodeQL/badge.svg)](https://github.com/datarhei/core/actions?query=workflow%3ACodeQL)
![Docker Pulls](https://img.shields.io/docker/pulls/datarhei/core.svg?maxAge=604800&label=Docker%20Pulls)

The cloud-native audio/video processing API.

datarhei Core is management for FFmpeg processes without development effort. It is a central interface for mapping AV processes, is responsible for design and management, and provides all necessary interfaces to access the video content. The included control for FFmpeg can keep all used functions reliable and executable without the need for software developers to take care of it. In addition, process and resource limitation for all FFmpeg processes protects the host system from application overload. The overall system gives access to current process values (CPU, RAM) and complete control of system resources and loads with statistical access to process data and current and historical logs.

## Features

-   Unrestricted FFmpeg process management
-   Optimized for long-running tasks
-   In-Memory- and Disk-Filesystem for media assets
-   HTTP and RTMP services
-   Let's Encrypt for HTTPS and RTMPS
-   HLS/DASH Session tracking with bandwidth and current viewer limiters
-   Multiple resource limiters and monitoring
-   FFmpeg progress data
-   Metrics incl. Prometheus support
-   Logging and debugging for FFmpeg processes with history
-   Multiple auth. by JWT and Auth0
-   100% JSON REST API (Swagger documented)
-   GraphQL for metrics, process, and progress data

## Quick start

1. Run the Docker image

```sh
docker run --name core -d
    -e CORE_API_AUTH_USERNAME=admin \
    -e CORE_API_AUTH_PASSWORD=secret \
    -p 8080:8080 \
    -v ${HOME}/core/config:/core/config \
    -v ${HOME}/core/data:/core/data \
    datarhei/core:latest
```

2. Open Swagger
   http://host-ip:8080/api/swagger/index.html

3. Log in with Swagger
   Authorize > Basic authorization > Username: admin, Password: secret

## Docker images

Native (linux/amd64,linux/arm64,linux/arm/v7)

-   datarhei/base:core-alpine-latest
-   datarhei/base:core-ubuntu-latest

Bundle with FFmpeg (linux/amd64,linux/arm64,linux/arm/v7)

-   datarhei/core:latest

Bundle with FFmpeg for Raspberry Pi (linux/arm/v7)

-   datarhei/core:rpi-latest

Bundle with FFmpeg for Nvidia Cuda (linux/amd64)

-   datarhei/core:cuda-latest

Bundle with FFmpeg for Intel VAAPI (linux/amd64)

-   datarhei/core:vaapi-latest

## Documentation

## Environment variables

The environment variables can be set in the file `.env`, e.g.

```
CORE_API_AUTH_USERNAME=admin
CORE_API_AUTH_PASSWORD=datarhei
...
```

You can also provide them on the command line, whatever you prefer. If the same environment variable is set
in the `.env` file and on the command line, the one set on the command line will overrule the one from the `.env` file.

The currently known environment variables (but not all will be respected) are:

| Name                                      | Default      | Description                                                                                                                                                                          |
| ----------------------------------------- | ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| CORE_CONFIGFILE                           | (not set)    | Path to a config file. The following environment variables will override the respective values in the config file.                                                                   |
| CORE_ADDRESS                              | `:8080`      | HTTP listening address.                                                                                                                                                              |
| CORE_LOG_LEVEL                            | `info`       | silent, error, warn, info, debug.                                                                                                                                                    |
| CORE_LOG_TOPICS                           | (not set)    | List of topics to log (comma separated)                                                                                                                                              |
| CORE_LOG_MAXLINES                         | `1000`       | Number of latest log lines to keep in memory.                                                                                                                                        |
| CORE_DB_DIR                               | `.`          | Directory for holding the operational data. This directory must exist.                                                                                                               |
| CORE_HOST_NAME                            | (not set)    | Set to the domain name of the host this instance is running on.                                                                                                                      |
| CORE_HOST_AUTO                            | `true`       | Enable detection of public IP addresses.                                                                                                                                             |
| CORE_API_READ_ONLY                        | `false`      | Allow only ready only access to the API                                                                                                                                              |
| CORE_API_ACCESS_HTTP_ALLOW                | (not set)    | Comma separated list of IP ranges in CIDR notation (HTTP traffic), e.g. `127.0.0.1/32,::1/128`.                                                                                      |
| CORE_API_ACCESS_HTTP_BLOCK                | (not set)    | Comma separated list of IP ranges in CIDR notation (HTTP traffic), e.g. `127.0.0.1/32,::1/128`.                                                                                      |
| CORE_API_ACCESS_HTTPS_ALLOW               | (not set)    | Comma separated list of IP ranges in CIDR notation (HTTPS traffic), e.g. `127.0.0.1/32,::1/128`.                                                                                     |
| CORE_API_ACCESS_HTTPS_BLOCK               | (not set)    | Comma separated list of IP ranges in CIDR notation (HTTPS traffic), e.g. `127.0.0.1/32,::1/128`.                                                                                     |
| CORE_API_AUTH_ENABLE                      | `true`       | Set to `false` to disable auth for all clients.                                                                                                                                      |
| CORE_API_AUTH_DISABLE_LOCALHOST           | `false`      | Set to `true` to disable auth for clients from localhost.                                                                                                                            |
| CORE_API_AUTH_USERNAME                    | (required)   | Username for auth.                                                                                                                                                                   |
| CORE_API_AUTH_PASSWORD                    | (required)   | Password for auth.                                                                                                                                                                   |
| CORE_API_AUTH_JWT_SECRET                  | (not set)    | A secret for en- and decrypting the JWT. If not set, a secret will be generated.                                                                                                     |
| CORE_API_AUTH_AUTH0_ENABLE                | `false`      | Enable Auth0.                                                                                                                                                                        |
| CORE_API_AUTH_AUTH0_TENANTS               | (not set)    | List of base64 encoded Auth0 tenant JSON objects (comma-separated). The tenant JSON object is defined as `{"domain":string,"audience":string,"users":array of strings}`              |
| CORE_TLS_ADDRESS                          | `:8181`      | Port to listen on for HTTPS requests.                                                                                                                                                |
| CORE_TLS_ENABLE                           | `false`      | Set to `true` to enable TLS support.                                                                                                                                                 |
| CORE_TLS_AUTO                             | `false`      | Set to `true` to enable automatic retrieval of a Let's Encrypt certificate. Requires `CORE_TLS_ENABLE` to be `true` and `CORE_HOST_NAME` to be set with `CORE_HOST_AUTO` to `false`. |
| CORE_TLS_CERTFILE                         | (not set)    | TLS certificate file in PEM format.                                                                                                                                                  |
| CORE_TLS_KEYFILE                          | (not set)    | TLS key file in PEM format.                                                                                                                                                          |
| CORE_STORAGE_DISK_DIR                     | `.`          | A directory that will be exposed by HTTP on /. This directory must exist.                                                                                                            |
| CORE_STORAGE_DISK_MAXSIZEMBYTES           | `0`          | Max. allowed megabytes for `CORE_STORAGE_DISK_DIR`.                                                                                                                                  |
| CORE_STORAGE_DISK_CACHE_ENABLE            | `true`       | Enable cache for files from `CORE_STORAGE_DISK_DIR`.                                                                                                                                 |
| CORE_STORAGE_DISK_CACHE_MAXSIZEMBYTES     | `0`          | Max. allowed cache size, 0 for unlimited.                                                                                                                                            |
| CORE_STORAGE_DISK_CACHE_TTLSECONDS        | `300`        | Seconds to keep files in cache.                                                                                                                                                      |
| CORE_STORAGE_DISK_CACHE_MAXFILESIZEMBYTES | `1`          | Max. file size to put in cache.                                                                                                                                                      |
| CORE_STORAGE_DISK_CACHE_TYPES             | (not set)    | List of file extensions to cache (space-separated, e.g. ".html .js"), empty for all.                                                                                                 |
| CORE_STORAGE_MEMORY_AUTH_ENABLE           | `true`       | Enable basic auth for PUT,POST, and DELETE on /memfs.                                                                                                                                |
| CORE_STORAGE_MEMORY_AUTH_USERNAME         | (not set)    | Username for Basic-Auth of `/memfs`. Required if auth is enabled.                                                                                                                    |
| CORE_STORAGE_MEMORY_AUTH_PASSWORD         | (not set)    | Password for Basic-Auth of `/memfs`. Required if auth is enabled.                                                                                                                    |
| CORE_STORAGE_MEMORY_MAXSIZEMBYTES         | `0`          | Max. allowed megabytes for `/memfs`. Any value <= 0 means unlimited.                                                                                                                 |
| CORE_STORAGE_MEMORY_PURGE                 | `false`      | Set to `true` to remove the oldest entries if the `/memfs` is full.                                                                                                                  |
| CORE_STORAGE_COCORE_ORIGINS               | `*`          | List of allowed CORS origins (comma separated). Will be used for `/` and `/memfs`.                                                                                                   |
| CORE_STORAGE_MIMETYPES_FILE               | `mime.types` | Path to file with MIME type definitions.                                                                                                                                             |
| CORE_RTMP_ENABLE                          | `false`      | Enable RTMP server.                                                                                                                                                                  |
| CORE_RTMP_ENABLE_TLS                      | `false`      | Enable RTMP over TLS (RTMPS). Requires `CORE_TLS_ENABLE` to be `true`.                                                                                                               |
| CORE_RTMP_ADDRESS                         | `:1935`      | RTMP server listen address.                                                                                                                                                          |
| CORE_RTMP_ADDRESS_TLS                     | `:1936`      | RTMPS server listen address.                                                                                                                                                         |
| CORE_RTMP_APP                             | `/`          | RTMP app for publishing.                                                                                                                                                             |
| CORE_RTMP_TOKEN                           | (not set)    | RTMP token for publishing and playing. The token is the value of the URL query parameter `token`.                                                                                    |
| CORE_SRT_ENABLE                           | `false`      | Enable SRT server.                                                                                                                                                                   |
| CORE_SRT_ADDRESS                          | `:6000`      | SRT server listen address.                                                                                                                                                           |
| CORE_SRT_PASSPHRASE                       | (not set)    | SRT passphrase.                                                                                                                                                                      |
| CORE_SRT_TOKEN                            | (not set)    | SRT token for publishing and playing. The token is the value of the URL query parameter `token`.                                                                                     |
| CORE_SRT_LOG_ENABLE                       | `false`      | Enable SRT server logging.                                                                                                                                                           |
| CORE_SRT_LOG_TOPICS                       | (not set)    | List topics to log from SRT server. See https://github.com/datarhei/gosrt#logging.                                                                                                   |
| CORE_FFMPEG_BINARY                        | `ffmpeg`     | Path to FFmpeg binary.                                                                                                                                                               |
| CORE_FFMPEG_MAXPROCESSES                  | `0`          | Max. allowed simultaneously running FFmpeg instances. Any value <= 0 means unlimited.                                                                                                |
| CORE_FFMPEG_ACCESS_INPUT_ALLOW            | (not set)    | List of pattern for allowed input URI (space-separated), leave emtpy to allow any.                                                                                                   |
| CORE_FFMPEG_ACCESS_INPUT_BLOCK            | (not set)    | List of pattern for blocked input URI (space-separated), leave emtpy to block none.                                                                                                  |
| CORE_FFMPEG_ACCESS_OUTPUT_ALLOW           | (not set)    | List of pattern for allowed output URI (space-separated), leave emtpy to allow any.                                                                                                  |
| CORE_FFMPEG_ACCESS_OUTPUT_BLOCK           | (not set)    | List of pattern for blocked output URI (space-separated), leave emtpy to block none.                                                                                                 |
| CORE_FFMPEG_LOG_MAXLINES                  | `50`         | Number of latest log lines to keep for each process.                                                                                                                                 |
| CORE_FFMPEG_LOG_MAXHISTORY                | `3`          | Number of latest logs to keep for each process.                                                                                                                                      |
| CORE_PLAYOUT_ENABLE                       | `false`      | Enable playout API where available                                                                                                                                                   |
| CORE_PLAYOUT_MINPORT                      | `0`          | Min. port a playout server per input can run on.                                                                                                                                     |
| CORE_PLAYOUT_MAXPORT                      | `0`          | Max. port a playout server per input can run on.                                                                                                                                     |
| CORE_DEBUG_PROFILING                      | `false`      | Set to `true` to enable profiling endpoint on `/profiling`.                                                                                                                          |
| CORE_DEBUG_FORCEGC                        | `0`          | Number of seconds between forcing GC to return memory to the OS. Use in conjuction with `GODEBUG=madvdontneed=1`. Any value <= 0 means not to force GC.                              |
| CORE_METRICS_ENABLE                       | `false`      | Enable collecting historic metrics data.                                                                                                                                             |
| CORE_METRICS_ENABLE_PROMETHEUS            | `false`      | Enable prometheus endpoint /metrics.                                                                                                                                                 |
| CORE_METRICS_RANGE_SECONDS                | `300`        | Seconds to keep history metric data.                                                                                                                                                 |
| CORE_METRICS_INTERVAL_SECONDS             | `2`          | Interval for collecting metrics.                                                                                                                                                     |
| CORE_SESSIONS_ENABLE                      | `false`      | Enable HLS statistics for `/memfs`.                                                                                                                                                  |
| CORE_SESSIONS_IP_IGNORELIST               | (not set)    | Comma separated list of IP ranges in CIDR notation, e.g. `127.0.0.1/32,::1/128`.                                                                                                     |
| CORE_SESSIONS_SESSION_TIMEOUT_SEC         | `30`         | Timeout of a session in seconds.                                                                                                                                                     |
| CORE_SESSIONS_PERSIST                     | `false`      | Whether to persist the session history. Will be stored in `CORE_DB_DIR`.                                                                                                             |
| CORE_SESSIONS_MAXBITRATE_MBIT             | `0`          | Max. allowed outgoing bitrate in mbit/s, 0 for unlimited.                                                                                                                            |
| CORE_SESSIONS_MAXSESSIONS                 | `0`          | Max. allowed number of simultaneous sessions, 0 for unlimited.                                                                                                                       |
| CORE_ROUTER_BLOCKED_PREFIXES              | `/api`       | List of path prefixes that can't be routed.                                                                                                                                          |
| CORE_ROUTER_ROUTES                        | (not set)    | List of route mappings of the form [from]:[to], e.g. `/foo:/bar`. Leave empty for no routings.                                                                                       |
| CORE_ROUTER_UI_PATH                       | (not set)    | Path to directory with files for a UI. It will be mounted to `/ui` and uses `index.html` as default index page.                                                                      |

## Config

The minimum config file has to look like this:

```
{
    "version": 1
}
```

All other values will be filled with default values and persisted on disk. The entire default config file:

```
{
    "version": 1,
    "id": "[will be generated if not given]",
    "name": "[will be generated if not given]",
    "address": ":8080",
    "log": {
        "level": "info",
        "topics": [],
        "max_lines": 1000
    },
    "db": {
        "dir": "./config"
    },
    "host": {
        "name": [],
        "auto": true
    },
    "api": {
        "read_only": false,
        "access": {
            "http": {
                "allow": [],
                "block": []
            },
            "https": {
                "allow": [],
                "block": []
            }
        },
        "auth": {
            "enable": true,
            "disable_localhost": false,
            "username": "",
            "password": "",
            "jwt": {
                "secret": ""
            },
            "auth0": {
                "enable": false,
                "tenants": []
            }
        }
    },
    "tls": {
        "address": ":8181",
        "enable": false,
        "auto": false,
        "cert_file": "",
        "key_file": ""
    },
    "storage": {
        "disk": {
            "dir": "./data",
            "max_size_mbytes": 0,
            "cache": {
                "enable": true,
                "max_size_mbytes": 0,
                "ttl_seconds": 300,
                "max_file_size_mbytes": 1,
                "types": []
            }
        },
        "memory": {
            "auth": {
                "enable": true,
                "username": "admin",
                "password": "vxbx0ViqfA75P1KCyw"
            },
            "max_size_mbytes": 0,
            "purge": false
        },
        "cors": {
            "origins": [
                "*"
            ]
        },
        "mimetypes_file": "mime.types"
    },
    "rtmp": {
        "enable": false,
        "enable_tls": false,
        "address": ":1935",
        "address_tls": ":1936",
        "app": "/",
        "token": ""
    },
    "srt": {
        "enable": false,
        "address": ":6000",
        "passphrase": "",
        "token": "",
        "log": {
            "enable": false,
            "topics": [],
        }
    },
    "ffmpeg": {
        "binary": "ffmpeg",
        "max_processes": 0,
        "access": {
            "input": {
                "allow": [],
                "block": []
            },
            "output": {
                "allow": [],
                "block": []
            }
        },
        "log": {
            "max_lines": 50,
            "max_history": 3
        }
    },
    "playout": {
        "enable": false,
        "min_port": 0,
        "max_port": 0
    },
    "debug": {
        "profiling": false,
        "force_gc": 0
    },
    "stats": {
        "enable": true,
        "ip_ignorelist": [
            "127.0.0.1/32",
            "::1/128"
        ],
        "session_timeout_sec": 30,
        "persist": false,
        "persist_interval_sec": 300,
        "max_bitrate_mbit": 0,
        "max_sessions": 0
    },
    "service": {
        "enable": false,
        "token": "",
        "url": "https://service.datarhei.com"
    },
    "router": {
        "blocked_prefixes": [
            "/api"
        ],
        "routes": {}
    }
}
```

If you don't provide a path to a config file, the default config will be used, and nothing will be persisted to the disk. Default values can be overruled by environment variables.

## TLS / HTTPS

Enable TLS / HTTPS support by setting `CORE_TLS_ENABLE=true` and provide the certificate file and key file in PEM format by setting the environment variables `CORE_TLS_CERTFILE` and `CORE_TLS_KEYFILE` accordingly. If a certificate authority signs the certificate, the certificate file should be the concatenation of the server's certificate, any intermediates, and the CA's certificate.

If TLS with given certificates is enabled, an HTTP server listening on `CORE_ADDRESS` (address) will be additionally started. This server provides access to the same memory filesystem as the HTTPS server (including limits and authorization), but its access is restricted to localhost only.

### Let's Encrypt

If you want to use automatic certificates from Let's Encrypt, set the environment variable `CORE_TLS_AUTO` to `true.` To work, the
environment variables `CORE_TLS_ENABLE` have to be `true,` and `CORE_HOST_NAME` has to be set to the host this host will be reachable. Otherwise, the ACME challenge will not work. The environment variables `CORE_TLS_CERTFILE` and `CORE_TLS_KEYFILE` will be ignored.

If automatic TLS is enabled, the HTTP server (CORE_ADDRESS, resp. address) must listen on port 80. It is required to automatically acquire the certificate (serving the `HTTP-01` challenge). As a further requirement, `CORE_HOST_NAME` (host.name) must be set because it is used a the canonical name for the certificate.

The obtained certificates will be stored in `CORE_DB_DIR/cert` to be available after a restart.

The obtained certificates will be stored in `CORE_DB_DIR/cert` to be available after a restart.

### Self-Signed certificates

To create a self-signed certificate and key file pair, run this command and provide a reasonable value for the Common Name (CN). The CN is the fully qualified name of the host the instance is running on (e.g., `localhost`). You can also use an IP address or a wildcard name, e.g., `*.example.com`.

RSA SSL certificate

```sh
openssl req -newkey rsa:2048 -nodes -keyout key.pem -x509 -days 365 -out cert.pem -subj '/CN=localhost'
```

ECDSA SSL certificate

```sh
openssl ecparam -name secp521r1 -genkey -out key.pem
openssl req -new -x509 -key key.pem -out cert.pem -days 365 -subj '/CN=localhost'
```

Call `openssl ecparam -list_curves` to see all available supported curves listed.

## Access Control

To control who has access to the API, a list of allowed IPs can be defined. This list is provided at startup with the environment variables `CORE_API_ACCESS_HTTP_BLOCK` and `CORE_API_ACCESS_HTTP_ALLOW.` This is a comma-separated list of IPs in CIDR notation,
e.g. `127.0.0.1/32,::1/128`. If the list is empty, then all IPs are allowed. If the list contains any invalid IP range, the server
will refuse to start. This can be separately defined for the HTTP and HTTPS server if you have TLS enabled with the environment variables `CORE_API_ACCESS_HTTPS_BLOCK` and `CORE_API_ACCESS_HTTPS_ALLOW.`

## Input/Output Control

To control where FFmpeg can read and where FFmpeg can write, you can define a pattern that matches the
input addresses or the output addresses. These patterns are regular expressions that can be provided at startup with the
environment variables `CORE_FFMPEG_ACCESS_INPUT` and `CORE_FFMPEG_ACCESS_OUTPUT.` The expressions need to be space-separated, e.g.
`HTTPS?:// RTSP:// RTMP://`. If one of the lists is empty, then no restriction on input, resp. The output will be applied.

Independently of the value of `CORE_FFMPEG_ACCESS_OUTPUT` there's a check that verifies that output can only be written to the specified `CORE_STORAGE_DISK_DIR` and works as follows: If the address has a protocol specifier other than `file:,` then no further checks will be applied. If the protocol is `file:` or no protocol specifier is given, the address is assumed to be a path that is checked against the path shown in `CORE_STORAGE_DISK_DIR.`

It will be rejected if the address is outside the `CORE_STORAGE_DISK_DIR` directory. Otherwise, the protocol `file:` will be prepended. If you give some expressions for `CORE_FFMPEG_ACCESS_OUTPUT,` you should also allow `file:.`

Special cases are the output addresses `-` (which will be rewritten to `pipe:`), and `/dev/null` (which will be allowed even though it's outside of `CORE_STORAGE_DISK_DIR`).

If you set a value for `CORE_STORAGE_DISK_CACHE_MAXSIZEMBYTES`, which is larger than `0`, it will be interpreted as max—allowed megabytes for the `CORE_STORAGE_DISK_DIR.` As soon as the limit is reached, all processes that have outputs writing to `CORE_STORAGE_DISK_DIR` will be stopped. You are responsible for cleaning up the directory and restarting these processes.

## RTMP

The datarhei Core includes a simple RTMP server for publishing and playing streams. Set the environment variable `CORE_RTMP_ENABLE` to `true` to enable the RTMP server. It is listening on `CORE_RTMP_ADDRESS`. Use `CORE_RTMP_APP` to limit the app a stream can be published on, e.g. `/live` to require URLs to start with `/live`. To prevent anybody can publish streams, set `CORE_RTMP_TOKEN` to a secret only known to the publishers and subscribers. The token has to be put in the query of the stream URL, e.g. `/live/stream?token=...`.

For additionaly enabling the RTMPS server, set the config variable `rtmp.enable_tls` or environment variable `CORE_RTMP_ENABLE_TLS` to `true`. This requires `tls.enable` or `CORE_TLS_ENABLE` to be set to to `true`. Use `rtmp.address_tls` or `CORE_RTMP_ADDRESS_TLS` to set the listen address for the RTMPS server.

| Method | Path         | Description                           |
| ------ | ------------ | ------------------------------------- |
| GET    | /api/v3/rtmp | List all currently published streams. |

## SRT

The datarhei Core includes a simple SRT server for publishing and playing streams. Set the environment variable `CORE_SRT_ENABLE` to `true` to enable the SRT server. It is listening on `CORE_SRT_ADDRESS`.

The `streamid` is formatted according to Appendix B of the [SRT specs](https://datatracker.ietf.org/doc/html/draft-sharabayko-srt#appendix-B). The following keys are supported:

| Key     | Descriptions                                                                                                      |
| ------- | ----------------------------------------------------------------------------------------------------------------- |
| `m`     | The connection mode, either `publish` for publishing a stream or `request` for subscribing to a published stream. |
| `r`     | Name of the resource.                                                                                             |
| `token` | A token to prevent anybody to publish or subscribe to a stream. This is set with `CORE_SRT_TOKEN`.                |

An example publishing streamid: `#!:m=publish,r=12345,token=foobar`.

With your SRT client, connect to the SRT server always in `caller` mode, e.g. `srt://127.0.0.1:6000?mode=caller&streamid=#!:m=publish,r=12345,token=foobar&passphrase=foobarfoobar&transmode=live`.

Via the API you can gather statistics of the currently connected SRT clients.

| Method | Path        | Description                           |
| ------ | ----------- | ------------------------------------- |
| GET    | /api/v3/srt | List all currently published streams. |

## Playout

FFmpeg processes with a `avstream:` (or `playout:`) input stream can expose an HTTP API to control the playout of that stream. With
`CORE_PLAYOUT_ENABLE` you enable exposing this API. The API is only exposed to `localhost` and is transparently connected to the datarhei Core API. You have to provide a port range (`CORE_PLAYOUT_MINPORT` and `CORE_PLAYOUT_MAXPORT`) where datarhei/core can use ports to assign it to the playout API.

| Method   | Path                                                   | Description                                                                                                                                                                                                                                                                                  |
| -------- | ------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| GET      | /api/v3/process/:id/playout/:inputid/status            | Retrieve the current status as JSON.                                                                                                                                                                                                                                                         |
| GET      | /api/v3/process/:id/playout/:inputid/keyframe/\*name   | Retrieve the last deliverd key frame from the input stream as JPEG (if `name` has the ending `.jpg`) or PNG (if `name` has the ending `.png`).                                                                                                                                               |
| GET      | /api/v3/process/:id/playout/:inputid/errorframe/encode | Immediately encode the error frame to a GOP. Will only have an effect if the last key frame is currently in a loop.                                                                                                                                                                          |
| PUT/POST | /api/v3/process/:id/playout/:inputid/errorframe/\*name | Upload any image or video media that can be decoded and will be used to replace the key frame loop. If the key frame is currently in a loop, it will be repaced immediately. Otherwise, it will be used the next time the key frame is in a loop. The body of the request is the media file. |
| PUT      | /api/v3/process/:id/playout/:inputid/stream            | Replace the current stream. The body of the request is the URL of the new stream.                                                                                                                                                                                                            |

## MIME Types

The file with the MIME types has one MIME type per line followed by a list of file extensions (including the ".").

```
text/plain  .txt
text/html   .htm .html
...
```

## Memory Filesystem

AA very simple in-memory filesystem is available. The uploaded data is stored in a map, where the path used to upload the file
is used as the key. Use the `POST` or `PUT` method with the proper direction for uploading a file. The body of the request contains the contents of the file. No particular encoding or `Content-Type` is required. The file can then be downloaded from the same path.

| Method | Path                 | Description                                                                                                                                                       |
| ------ | -------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| POST   | /memfs/\*path        | Upload a file to the memory filesystem. The filename is `path` which can contain slashes. If there's already a file with the same `path`, it will be overwritten. |
| PUT    | /memfs/\*path        | Same as POST.                                                                                                                                                     |
| GET    | /memfs/\*path        | Download the file stored under `path`. The MIME types are applied based on the extension in the `path`.                                                           |
| DELETE | /memfs/\*path        | Delete the file stored under `path`.                                                                                                                              |
| POST   | /api/v3/memfs/\*path | Upload a file to the memory filesystem.                                                                                                                           |
| PUT    | /api/v3/memfs/\*path | Same as POST.                                                                                                                                                     |
| GET    | /api/v3/memfs/\*path | Download the file stored under `path`.                                                                                                                            |
| PATCH  | /api/v3/memfs/\*path | Create a link to a file. The body contains the path to that file.                                                                                                 |
| DELETE | /api/v3/memfs/\*path | Delete the file stored under `path`.                                                                                                                              |
| GET    | /api/v3/memfs        | List all files that are currently stored in the in-memory filesystem.                                                                                             |

Use these endpoints to, e.g., store HLS chunks and .m3u8 files (in contrast to an actual disk or a ramdisk):

```
ffmpeg -f lavfi -re -i testsrc2=size=640x480:rate=25 -c:v libx264 -preset:v ultrafast -r 25 -g 50 -f hls -start_number 0 -hls_time 2 -hls_list_size 6 -hls_flags delete_segments+temp_file+append_list -method PUT -hls_segment_filename http://localhost:8080/memfs/foobar_%04d.ts -y http://localhost:8080/memfs/foobar.m3u8
```

Then you can play it generally with, e.g., `ffplay http://localhost:3000/memfs/foobar.m3u8`.

Use the environment variables `CORE_STORAGE_MEMORY_AUTH_USERNAME` and `CORE_STORAGE_MEMORY_AUTH_PASSWORD` to protect the `/memfs` with Basic-Auth. Basic-Auth will only be enabled
if both environment variables are set to non-empty values. The `GET /memfs/:path` will not be protected with Basic-Auth.

Use the environment variable `CORE_STORAGE_MEMORY_MAXSIZEMBYTES` to limit the amount of data that is allowed to be stored. The value is interpreted as megabytes. Use a value equal to or smaller than `0` not to impose any limits. A `507 Insufficient Storage` will be returned if you hit the limit.

Listing all currently stored files is done by calling `/v3/memfs` with the credentials set by the environment variables `CORE_API_AUTH_USERNAME` and `CORE_API_AUTH_PASSWORD`.
It also accepts the query parameter `sort` (`name,` `size,` or `lastmod`) and `order` (`asc` or `desc`). If a valid value for `sort` is given, the results are sorted in ascending order.

## Routes

All contents in `CORE_STORAGE_DISK_DIR` are served from `/.` If you want to redirect some paths to an existing file, you can add static routes in `router.routes` by providing a direct mapping, e.g.

```
router: {
    routes: {
        "/foo.txt": "/bar.txt",
    }
}
```

The paths have to start with a `/.` Alternatively, you can serve whole directories from another root than `CORE_STORAGE_DISK_DIR.` Use a `/*` at the end of a path as key and a path on the filesystem as the target, e.g.

```
router: {
    routes: {
        "/ui/*": "/path/to/ui",
    }
}
```

If you use a relative path as target, then it will be added to the current working directory.

## API

Check the detailed API description on `/api/swagger/index.html`.

### Login / Auth

With auth enabled, you have to retrieve a JWT/OAuth token before you can access the `/v3/` API calls.

| Method | Path                  | Description                                    |
| ------ | --------------------- | ---------------------------------------------- |
| POST   | /api/login            | Retrieve a token to access the API.            |
| GET    | /api/v3/refresh_token | Retrieve a fresh token with a new expiry date. |

For the login you have to send

```
{
    "username": "...",
    "password": "..."
}
```

The `username` and the `password` are set by the environment variables `CORE_API_AUTH_USERNAME` and `CORE_API_AUTH_PASSWORD`.

On successful login, the response looks like this:

```
{
    "expire": "2019-01-18T19:55:55+01:00",
    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE1NDc4Mzc3NTUsImlkIjpudWxsLCJvcmlnX2lhdCI6MTU0NzgzNDE1NX0.ZcrpD4oRBqG3wUrfnh1DOVpXdUT7dvUnvetKFEVRKKc"
}
```

Use the `token` in all subsequent calls to the `/api/v3/` endpoints, e.g.

```
http http://localhost:8080/api/v3/process "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE1NDc4Mzc3NTUsImlkIjpudWxsLCJvcmlnX2lhdCI6MTU0NzgzNDE1NX0.ZcrpD4oRBqG3wUrfnh1DOVpXdUT7dvUnvetKFEVRKKc"
```

### Config

| Method | Path                  | Description                                                                                                          |
| ------ | --------------------- | -------------------------------------------------------------------------------------------------------------------- |
| GET    | /api/v3/config        | Retrieve the current config without the override values from the environment variables.                              |
| GET    | /api/v3/config/active | Retrieve the current config with the override values from the environment variables are taken into account.          |
| PUT    | /api/v3/config        | Store a new config. Only some values are respected, and the new config will only be used after a restart.            |
| GET    | /api/v3/config/reload | Reload the config. The config will be re-read and validated from the store. It will cause a restart of all services. |

When retrieving the config via the API, critical values (such as passwords) will be disguised if not required otherwise.

### Process

With the process API call, you can manage different FFmpeg processes. A process is defined as:

```
{
    "id": "SomeId",
    "reference": "SomeId",
    "type": "ffmpeg",
    "input": [
        {
            "id": "inputid",
            "address": "rtsp://1.2.3.4/stream.sdp",
            "options": [
                ... list of input options ...
            ]
        },
        ... list of inputs ...
    ],
    "output": [
        {
            "id": "outputid",
            "address": "rtmp://rtmp.youtube.com/live2/...",
            "options": [
                ... list of output options ...
            ],
            "cleanup": [{
                "pattern": "(memfs|diskfs):...",
                "max_files: "number,
                "max_file_age_seconds": "number",
                "purge_on_delete: "(true|false)"
            }]
        },
        ... list of outputs ...
    ],
    "options": [
        ... list of global options ...
    ],
    "reconnect": (true|false),
    "reconnect_delay_seconds": 10,
    "autostart": (true|false),
    "stale_timeout_seconds": 30
}
```

The input, output, and global options are interpreted as command-line options for FFmpeg.

#### Process Cleanup

With the optional array of cleanup rules for each output, it is possible to define rules for removing files from the
memory filesystem or disk. Each rule consists of a glob pattern and a max. allowed number of files matching that pattern or
permitted maximum age for the files matching that pattern. The pattern starts with either `memfs:` or `diskfs:` depending on
which filesystem this rule is designated to. Then a [glob pattern](https://pkg.go.dev/path/filepath#Match) follows to
identify the files. If `max_files` is set to a number > 0, then the oldest files from the matching files will be deleted if
the list of matching files is longer than that number. If `max_file_age_seconds` is set to a number > 0, then all files
that are older than this number of seconds from the matching files will be deleted. If `purge_on_delete` is set to `true`,
then all matching files will be deleted when the process is deleted.

The API calls are

| Method | Path                          | Description                                                                                                                                                                                                                                                                                                                                                                               |
| ------ | ----------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| POST   | /api/v3/process               | Adds a process. Overwriting an existing ID will result in an error.                                                                                                                                                                                                                                                                                                                       |
| GET    | /api/v3/process               | Retrieve a list of all known processes. Use the query parameter `ids` to list (comma separated) the IDs of the process you want to be part of the response. If the list is empty, all processes will be listed. Use the query parameter `filter` to list (comma separated) the wanted details per process (`config`, `state`, `log`). If the list is empty, all details will be included. |
| GET    | /api/v3/process/:id           | Retreive the details of a process including the config, state, and logs. Use the query parameter `filter` to list (comma separated) the wanted details per process (`config`, `state`, `log`). If the list is empty, all details will be included.                                                                                                                                        |
| PUT    | /api/v3/process/:id           | Replaces the process with a new config.                                                                                                                                                                                                                                                                                                                                                   |
| GET    | /api/v3/process/:id/config    | Retrieve the config of a process as it was provided.                                                                                                                                                                                                                                                                                                                                      |
| GET    | /api/v3/process/:id/state     | Retrieve the current state of a process. This includes the progress data if the process is running.                                                                                                                                                                                                                                                                                       |
| GET    | /api/v3/process/:id/report    | Retrieve the report and logs of a process.                                                                                                                                                                                                                                                                                                                                                |
| GET    | /api/v3/process/:id/debug     | Retrieve an anonymized version of the details of a process.                                                                                                                                                                                                                                                                                                                               |
| DELETE | /api/v3/process/:id           | Remove a specific process. Only possible if the process is not running.                                                                                                                                                                                                                                                                                                                   |
| PUT    | /api/v3/process/:id/command   | Send a command to a process.                                                                                                                                                                                                                                                                                                                                                              |
| GET    | /api/v3/process/:id/data      | Get all arbitrary JSON data that is stored with this process.                                                                                                                                                                                                                                                                                                                             |
| GET    | /api/v3/process/:id/data/:key | Get arbitrary JSON data that is stored under the key `key.`                                                                                                                                                                                                                                                                                                                               |
| PUT    | /api/v3/process/:id/data/:key | Store aribtrary JSON data under the key `key.` If the data is `null,` the key will be removed.                                                                                                                                                                                                                                                                                            |

### Commands

A command is defined as:

```
{
    "command": ("start"|"stop"|"restart"|"reload")
}
```

| Command   | Description                                                                                    |
| --------- | ---------------------------------------------------------------------------------------------- |
| `start`   | Start the process. If the process is already started, this won't have any effect.              |
| `stop`    | Stop the process. If the process is already stopped, this won't have any effect.               |
| `restart` | Restart the process. If the process is not running, this won't have any effect.                |
| `reload`  | Reload the process. If the process was running, the reloaded process will start automatically. |

### Placeholder

Currently supported placeholders are:

| Placeholder   | Description                                                                                                                                            | Location                                                                                                                |
| ------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------- |
| `{diskfs}`    | Will be replaced by the provided `CORE_STORAGE_DISK_DIR`.                                                                                              | `options`, `input.address`, `input.options`, `output.address`, `output.options`                                         |
| `{memfs}`     | Will be replace by the base URL of the MemFS.                                                                                                          | `input.address`, `input.options`, `output.address`, `output.options`                                                    |
| `{processid}` | Will be replaced by the ID of the process.                                                                                                             | `input.id`, `input.address`, `input.options`, `output.id`, `output.address`, `output.options`, `output.cleanup.pattern` |
| `{reference}` | Will be replaced by the reference of the process                                                                                                       | `input.id`, `input.address`, `input.options`, `output.id`, `output.address`, `output.options`, `output.cleanup.pattern` |
| `{inputid}`   | Will be replaced by the ID of the input.                                                                                                               | `input.address`, `input.options`                                                                                        |
| `{outputid}`  | Will be replaced by the ID of the output.                                                                                                              | `output.address`, `output.options`, `output.cleanup.pattern`                                                            |
| `{rtmp}`      | Will be replaced by the internal address of the RTMP server. Requires parameter `name` (name of the stream).                                           | `input.address`, `output.address`                                                                                       |
| `{srt}`       | Will be replaced by the internal address of the SRT server. Requires parameter `name` (name of the stream) and `mode` (either `publish` or `request`). | `input.address`, `output.address`                                                                                       |

Before replacing the placeholders in the process config, all references (see below) will be resolved.

If the value that gets filled in on the place of the placeholder needs escaping, you can define the character to be escaped in the placeholder by adding it to the placeholder name and prefix it with a `^`.
E.g. escape all `:` in the value (`http://example.com:8080`) for `{memfs}` placeholder, write `{memfs^:}`. It will then be replaced by `http\://example.com\:8080`. The escape character is always `\`. In
case there are `\` in the value, they will also get escaped. If the placeholder doesn't imply escaping, the value will be uses as-is.

Add parameters to a placeholder by appending a comma separated list of key/values, e.g. `{placeholder,key1=value1,key2=value2}`. This can be combined with escaping.

### References

The input address of a process may contain a reference to the output of another process. It has the form `#[processid]:output=[id]`.

A reference starts with a `#` followed by the process ID it refers to, followed by a `:.` Then comes `output,` followed by a `=.`
and the ID of the output.

## FFmpeg

### Statistics

This repository contains a patch for the FFmpeg program to provide detailed progress information. With this patch, FFmpeg will output
the progress information in a JSON string that contains the data for each input and output stream individually. The JSON output is enabled
by default. It can be enabled with the global `-jsonstats` switch on the command line. Use the `-stats` switch
on the command line for the standard progress output.

The Docker image that you can build with the provided Dockerfile includes the patched version of FFmpeg for your convenience.

Example output with `-stats`:

```
frame=  143 fps= 25 q=-1.0 Lsize=     941kB time=00:00:05.68 bitrate=1357.0kbits/s speed=0.995x
```

Example output with `-jsonstats`:

```
JSONProgress:{"inputs":[{"id":0, "stream":0, "type":"video", "codec":"rawvideo", "coder":"rawvideo", "pix_fmt":"rgb24", "frame":188, "fps":24.95, "width":1280, "height":720, "size_kb":507600, "bitrate_kbps":552960.0},{"id":1, "stream":0, "type":"audio", "codec":"pcm_u8", "coder":"pcm_u8", "frame":314, "sampling_hz":44100, "layout":"stereo", "size_kb":628, "bitrate_kbps":705.6}], "outputs":[{"id":0, "stream":0, "type":"video", "codec":"h264", "coder":"libx264", "pix_fmt":"yuv420p", "frame":188, "fps":24.95, "q":-1.0, "width":1280, "height":720, "size_kb":1247, "bitrate_kbps":1365.6},{"id":0, "stream":1, "type":"audio", "codec":"aac", "coder":"aac", "frame":315, "sampling_hz":44100, "layout":"stereo", "size_kb":2, "bitrate_kbps":2.1}], "frame":188, "fps":24.95, "q":-1.0, "size_kb":1249, "bitrate_kbps":1367.7, "time":"0h0m7.48s", "speed":0.993, "dup":0, "drop":0}
```

The same output but nicely formatted:

```json
{
    "bitrate_kbps": 1367.7,
    "drop": 0,
    "dup": 0,
    "fps": 24.95,
    "frame": 188,
    "inputs": [
        {
            "bitrate_kbps": 552960.0,
            "codec": "rawvideo",
            "coder": "rawvideo",
            "fps": 24.95,
            "frame": 188,
            "height": 720,
            "id": 0,
            "pix_fmt": "rgb24",
            "size_kb": 507600,
            "stream": 0,
            "type": "video",
            "width": 1280
        },
        {
            "bitrate_kbps": 705.6,
            "codec": "pcm_u8",
            "coder": "pcm_u8",
            "frame": 314,
            "id": 1,
            "layout": "stereo",
            "sampling_hz": 44100,
            "size_kb": 628,
            "stream": 0,
            "type": "audio"
        }
    ],
    "outputs": [
        {
            "bitrate_kbps": 1365.6,
            "codec": "h264",
            "coder": "libx264",
            "fps": 24.95,
            "frame": 188,
            "height": 720,
            "id": 0,
            "pix_fmt": "yuv420p",
            "q": -1.0,
            "size_kb": 1247,
            "stream": 0,
            "type": "video",
            "width": 1280
        },
        {
            "bitrate_kbps": 2.1,
            "codec": "aac",
            "coder": "aac",
            "frame": 315,
            "id": 0,
            "layout": "stereo",
            "sampling_hz": 44100,
            "size_kb": 2,
            "stream": 1,
            "type": "audio"
        }
    ],
    "q": -1.0,
    "size_kb": 1249,
    "speed": 0.993,
    "time": "0h0m7.48s"
}
```

### Resilient Streaming

Prepend the input source with `avstream:`, e.g. `... -i avstream:rtsp://1.2.3.4/stream.sdp ...`. It will reconnect to the stream if it breaks and repeats the last known intraframe until new data from the input stream is available.

## Example

Start `core` with the proper environment variables. Create a `.env` file or provide them on the command line. For this example, please use the following command line:

```
env CORE_API_AUTH_USERNAME=admin CORE_API_AUTH_PASSWORD=datarhei CORE_LOGLEVEL=debug CORE_STORAGE_DISK_DIR=./data ./core
```

Also, make sure that the directory `./data` exists. Otherwise, the state will not be stored and will be lost after a restart of
datarhei/core and the FFmpeg process will not be able to write the files.

In this example, we will add a fake video and audio source. The video will be encoded with H264, and the audio will be encoded with AAC. The output will be an m3u8 stream.

To talk to the API, we use the program [httpie](https://httpie.org/).

First, we create a JSON file with the process definition (e.g. `testsrc.json`):

```json
{
    "id": "testsrc",
    "type": "ffmpeg",
    "options": ["-loglevel", "info", "-err_detect", "ignore_err"],
    "input": [
        {
            "address": "testsrc=size=1280x720:rate=25",
            "id": "video",
            "options": ["-f", "lavfi", "-re"]
        },
        {
            "address": "anullsrc=r=44100:cl=stereo",
            "id": "audio",
            "options": ["-f", "lavfi"]
        }
    ],
    "output": [
        {
            "address": "http://127.0.0.1:8080/memfs/{processid}_{outputid}.m3u8",
            "id": "hls",
            "options": [
                "-codec:v",
                "libx264",
                "-preset:v",
                "ultrafast",
                "-r",
                "25",
                "-g",
                "50",
                "-pix_fmt",
                "yuv420p",
                "-b:v",
                "1024k",
                "-codec:a",
                "aac",
                "-b:a",
                "64k",
                "-hls_time",
                "2",
                "-hls_list_size",
                "10",
                "-hls_flags",
                "delete_segments+temp_file+append_list",
                "-hls_segment_filename",
                "http://127.0.0.1:8080/memfs/{processid}_{outputid}_%04d.ts"
            ]
        }
    ],
    "reconnect": true,
    "reconnect_delay_seconds": 10,
    "stale_timeout_seconds": 10
}
```

and POST it to the API:

```
http POST http://localhost:8080/v3/process < testsrc.json
```

Then check if it is there (as provided)

```
http http://localhost:8080/v3/process/testsrc
```

For the advanced, create another JSON file (e.g. `dump.json`):

```json
{
    "id": "dump",
    "type": "ffmpeg",
    "options": ["-loglevel", "info", "-err_detect", "ignore_err"],
    "input": [
        {
            "address": "#testsrc:output=hls",
            "id": "video",
            "options": []
        }
    ],
    "output": [
        {
            "address": "{diskfs}/{processid}.mp4",
            "id": "hls",
            "options": ["-codec", "copy", "-y"]
        }
    ],
    "reconnect": true,
    "reconnect_delay_seconds": 10,
    "stale_timeout_seconds": 10
}
```

and POST it to the API:

```
http POST http://localhost:8080/v3/process < dump.json
```

Then check if it is there (as provided)

```
http http://localhost:8080/v3/process/dump
```

Let's start the `testsrc` process

```
http PUT http://localhost:8080/v3/process/testsrc/command command=start
```

Now we can observe the progress of the process

```
http http://localhost:8080/v3/process/testsrc
```

or the log of the process

```
http http://localhost:8080/v3/process/testsrc/log
```

If you want to change the video bitrate, edit the `testsrc.json` file accordingly and replace the process:

```
http PUT http://localhost:8080/v3/process/testsrc < testsrc.json
```

It will stop the process, replace the config, and restart it.

Now open, e.g., VLC, and load the stream `http://localhost:8080/memfs/testsrc_hls.m3u8`.

This is enough; let's stop it

```
http PUT http://localhost:8080/v3/process/testsrc/command command=stop
```

and check its progress again

```
HTTP http://localhost:8080/v3/process/testsrc
```

Delete the process

```
HTTP DELETE http://localhost:8080/v3/process/testsrc
```

## Metrics

Metrics for the processes and other aspects are provided for a Prometheus scraper on `/metrics.`

Currently, available metrics are:

| Metric                | Type    | Dimensions                                                | Description                                     |
| --------------------- | ------- | --------------------------------------------------------- | ----------------------------------------------- |
| ffmpeg_process        | gauge   | `core`, `process`, `name`                                 | General stats per process.                      |
| ffmpeg_process_io     | gauge   | `core`, `process`, `type`, `id`, `index`, `media`, `name` | Stats per input and output of a process.        |
| mem_limit_bytes       | gauge   | `core`                                                    | Total available memory in bytes.                |
| mem_free_bytes        | gauge   | `core`                                                    | Free memory in bytes.                           |
| net_rx_bytes          | gauge   | `core`, `interface`                                       | Number of received bytes by interface.          |
| net_tx_bytes          | gauge   | `core`, `interface`                                       | Number of sent bytes by interface.              |
| cpus_system_time_secs | gauge   | `core`, `cpu`                                             | System time per CPU in seconds.                 |
| cpus_user_time_secs   | gauge   | `core`, `cpu`                                             | User time per CPU in seconds.                   |
| cpus_idle_time_secs   | gauge   | `core`, `cpu`                                             | Idle time per CPU in seconds.                   |
| session_total         | counter | `core`, `collector`                                       | Total number of sessions by collector.          |
| session_active        | gauge   | `core`, `collector`                                       | Current number of active sessions by collector. |
| session_rx_bytes      | counter | `core`, `collector`                                       | Total received bytes by collector.              |
| session_tx_bytes      | counter | `core`, `collector`                                       | Total sent bytes by collector.                  |

## Profiling

Profiling information is available under `/profiling.` Set the environment variable `CORE_DEBUG_PROFILING=true` to make this endpoint
available. If authentication is enabled, you have to provide the token in the header.

## Development

### Requirement

-   Go v1.18+ ([Download here](https://golang.org/dl/))

### Build

Clone the repository and build the binary

```
git clone git@github.com:datarhei/core.git
cd core
make
```

After the build process, the binary is available as `core`

For more build options, run `make help.`

### Cross Compile

If you want to run the binary on a different operating system and/or architecture, you create the appropriate binary by simply setting some
environment variables, e.g.

```
env GOOS=linux GOARCH=arm go build -o core-linux-arm
env GOOS=linux GOARCH=arm64 go build -o core-linux-arm64
env GOOS=freebsd GOARCH=amd64 go build -o core-freebsd-amd64
env GOOS=windows GOARCH=amd64 go build -o core-windows-amd64
env GOOS=macos GOARCH=amd64 go build -o core-macos-amd64
...
```

### Docker

Build the Docker image and run it to try out the API

```
docker build -t core .
docker run -it --rm -v ${PWD}/data:/core/data -p 8080:8080 core
```

### API Documentation

The documentation of the API is available on `/api/swagger/index.html.`

To generate the API documentation from the code, use [swag](https://github.com/swaggo/swag).

```
go install github.com/swaggo/swag (unless already installed)
make swagger
```

### Code style

The source code is formatted with `go fmt`, or simply run `make fmt`. Static analysis of the source code is done with `staticcheck`
(see [staticcheck](https://staticcheck.io/docs/)), or simply run `make lint`.

Before committing changes, you should run `make commit` to ensure that the source code is in shape.

## License

datarhei/core is licensed under the Apache License 2.0
