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

## Restrict alias patterns

Alias patterns are full regular expressions. The prefix pre-check in `match()` only helps
patterns that are anchored at the beginning and case-sensitive, and `(?m)^…` also matches after
a newline in a decoded path. Idea: `compile()` (`alias/alias.go`) only accepts patterns that

- are anchored at the beginning: the parsed pattern (`regexp/syntax`) is `OpBeginText` or an
  `OpConcat` that starts with `OpBeginText` (`^` or `\A`). This also rejects a top-level
  alternation like `^/a|/b`, which has to be written as `^(?:/a|/b)`.
- contain no flags: every `(?…` group is rejected except `(?:`, `(?P<name>` and `(?<name>`.
  A small scanner has to skip escaped characters and character classes (`\(`, `[(?i)]`).

Every pattern then gets the prefix pre-check, and it prepares the prefix index above. Aliases
are not released yet, so no stored rules need to be migrated as long as this is done before the
next release. `load()` and the cluster aliaser already skip invalid stored rules with a warning.
