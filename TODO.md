# TODO

## `util.PathParam` decodes path parameters twice

`PathParam` in `http/handler/util/util.go` calls `url.PathUnescape` on `c.Param(name)`.
Echo routes on `URL.RawPath` only if it is set, otherwise on the already decoded
`URL.Path`. `RawPath` is only set if the path contains characters whose encoding differs
from the default encoding, e.g. `%2F`. So in most cases the parameter is already decoded
and gets decoded a second time:

- `PUT /api/v3/fs/mem/a%2520b.txt` stores the file as `a b.txt` instead of `a%20b.txt`.
- A path with an encoded lone `%` (`%25`) fails to decode and becomes an empty path.

Affected are all callers, e.g. the filesystem handlers via `PathWildcardParam`, process IDs,
user names, and the cluster handlers. A fix must decode only if the route was matched on
`RawPath`, or always route on `RawPath` (see echo's `GetPath`).

## process with schedule time in the past prevents relocation

Needs investigation
