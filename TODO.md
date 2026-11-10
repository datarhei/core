# TODO

## `util.DefaultQuery` decodes query values twice

`DefaultQuery` in `http/handler/util/util.go` calls `url.QueryUnescape` on the value
returned by `c.QueryParam`, which echo has already decoded. Values are decoded twice:

- A `+` becomes a space, e.g. a regular expression `.+` turns into `. `.
- An encoded `%`, e.g. `%2520` sent for a literal `%20`, ends up as a space.
- A value with a lone `%` fails to decode and silently falls back to the default value.

Affected are all callers, e.g. glob patterns in `http/handler/filesystem.go` and
`http/handler/api/cluster_fs.go`, filters in `http/handler/api/process.go`, and the
handlers in `cluster/api.go`.

Fix: return `c.QueryParam(name)` without decoding it again. Check whether any client
relies on the double decoding. The alias handler (`http/handler/api/alias.go`) already
reads the parameter directly.
