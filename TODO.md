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
