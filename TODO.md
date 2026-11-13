# TODO

## Further optimizations for matching aliases

`match()` in `alias/alias.go` is called for every GET and HEAD request to a filesystem. Rules
whose literal prefix doesn't match are already skipped (about 3 ns per rule), but the cost is
still linear in the number of rules (`BenchmarkMatch`: about 3 ms for 1000000 rules). Options:

- Index the rules by their literal prefix (trie or map by path segment), such that only the
  rules whose prefix matches the path and the rules without a prefix are evaluated, in their
  original order. A path without a match then costs about the same regardless of the number
  of rules. Only worth it with tens of thousands of rules.
- Cache the results (path → target or no alias), bounded in size and cleared when the rules
  change. Helps paths that are requested repeatedly (e.g. playlists), but not segments.
- Call `MatchString` first and `FindStringSubmatchIndex` only on a match. Small gain for rules
  without a literal prefix.

## Glob syntax for alias patterns

Replace the regular expressions of alias patterns with the glob syntax that IAM resources use
(`glob` package, `github.com/gobwas/glob` with `/` as separator), e.g.
`/memfs/live/*.m3u8` → `/memfs/live/$1_main.m3u8`. Aliases are not released yet, so no stored
rules need to be migrated as long as this is done before the next release.

- Validate the pattern with `glob.Compile(pattern, '/')`. This catches structural errors like an
  unclosed `{`.
- Translate the tokens of `syntax.NewLexer` (exported by gobwas/glob) to a regular expression
  that is anchored with `^…$`:
  - `Text` → `regexp.QuoteMeta`;
  - `*` → `([^/]*)`, `**` → `(.*)`, `?` → `([^/])`;
  - `[…]` / `[!…]` → `([…])` / `([^…])`;
  - `{a,b}` → `(a|b)`.
  The rest of `alias` stays as it is (`match()`, `ExpandString`, prefix pre-check).
- Every wildcard and every `{…}` group is a capture group, numbered from the left in the order
  they open. The target references them with `$1` or `${1}`. Named references are not available.
- Check that the target only references existing groups (`$n` with n ≤ `NumSubexp()`). Today
  `ExpandString` silently replaces unknown references with an empty string.
- Match the semantics of gobwas/glob exactly, e.g. whether `[!…]` and `?` match `/`. Add a
  test that compares the translated regexp with `glob.Match` for a set of patterns and paths.
- The prefix pre-check then applies to every pattern, because the literal prefix is everything up
  to the first wildcard (`glob.Prefix`). That also simplifies the prefix index above.
- Update the doc comments of `alias.Rule` and `api.Alias`, then regenerate the swagger files.

Downside: less expressive than regular expressions, e.g. no `\d+`, no repetition counts, and no
optional parts other than `{a,}`.

## HEAD requests to filesystems read the whole file

The global `autohead` middleware (`http/middleware/autohead/autohead.go`) turns every HEAD
request into a GET with a writer that discards the body. `GetFile` (`http/handler/filesystem.go`)
then streams the whole file. For a large file on disk or S3 this reads the whole file for nothing.

`autohead` is needed, because some middleware changes the body, and HEAD has to report the same
headers (e.g. `Content-Length`) as GET:

- `.m3u8`: `hlsrewrite` (disk only) rewrites the paths, and the session middleware
  (`handleHLSEgress`) adds the session IDs and sets a new `Content-Length`.
- Content types in `compress.mimetypes` with `Accept-Encoding` and at least `compress.min_length`
  bytes: the compress middleware compresses the body, removes `Content-Length` and adds
  `Content-Encoding`.

Other files, e.g. segments (`.ts`, `.mp4`, `.m4s`), videos and images, are not changed, and the
size from `Stat()` would be correct. Simply skipping `autohead` for the filesystem routes is
therefore wrong. Possible directions:

- `autohead` skips paths that are not `.m3u8` and whose content type (from the mime middleware)
  is not compressible. This duplicates the conditions of the other middlewares.
- HEAD goes to `GetFile` directly, and only the middlewares that change the body (hlsrewrite,
  session for `.m3u8`, compress) turn HEAD into GET internally, and only if they would change
  the body. Each middleware knows its own conditions, but three middlewares have to be changed.

## memfs copies the file data on every open

`Open()` of the memory filesystem calls `LoadAndCopy` (`io/fs/mem_storage.go`), which copies the
file's data into a new buffer for every request. Each running download of a large file holds
another copy until it finishes. The copy is probably there because the buffers are pooled
(`mem.Get`/`mem.Put`) and can be reused after the file is overwritten or removed. Sharing the
data read-only would need reference counting of the buffers.
