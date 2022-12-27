# Unbound

A wrapper for Unbound in Go.

Unbound's `ub_result` has been extended with an slice of dns.RRs, this alleviates
the need to parse `ub_result.data` yourself.

The website for Unbound is https://unbound.net/, where you can find further documentation.

Tested/compiled to work for versions: 1.4.22 and 1.6.0-3+deb9u1 (Debian Stretch).

Note: using cgo means the executables will use shared libraries (OpenSSL, ldns and libunbound).

The tutorials found here are the originals ones adapted to Go.
