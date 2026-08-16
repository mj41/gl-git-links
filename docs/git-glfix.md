# git-glfix

Maintain gl: links in a git repository by tracking line changes.

## Synopsis

```
git-glfix [options] [command [args]]
```

## Description

git-glfix tracks and updates gl: links (e.g., `gl:path/to/file#L123`) as files change over time. It records snapshots in the repository and uses git history to adjust line references.

Only links carrying a line spec are tracked. A bare `gl:path/to/file` has no
line number to maintain, so git-glfix neither reports nor rewrites it. The line
spec must match the grammar in [the gl: specification](spec/gl-spec.md): `#L42`
or `#L42-L58`, first digit non-zero — `#L0` and `#L007` are not line links.

Links are found in every tracked, non-binary file, source and prose alike, so a
link in a code comment is maintained the same way as one in Markdown.

## Options

- `--dry-run` – Print what would be changed without modifying files.
- `--verbose` – Show detailed tracking information.
- `--json` – Output results in JSON format.
- `--update-modified` – Update links even if source files have uncommitted changes.
- `--version` – Print version, commit and build information.
- `--help` – Print usage.

## Line ranges

A link may address a single line or a range of lines:

```
gl:docs/spec/gl-spec.md#L59        single line
gl:docs/spec/gl-spec.md#L59-L74    range
```

Both ends of a range are tracked independently, so a range widens or narrows as
lines are inserted or deleted inside it, and moves as a whole when lines are
added above it. A range whose start or end line is deleted is reported as
broken, like a single-line link.

With `--json`, a range reports `old_end_line` and `new_end_line` alongside
`old_line` and `new_line`. Both are omitted for single-line links.

## Commands

With no command, git-glfix updates every tracked link in the repository.

- `status` – Show repository and snapshot status.
- `validate` – Scan for broken links without making changes. Exits non-zero if
  any link is broken.
- `help [<command>]` – Usage, for the tool or for one command.
- `cache <subcommand>` – Manage the snapshot cache. Subcommands: `list`, `show <commit>`, `clear`, `prune`, `rebuild`.
- `config <subcommand>` – View or modify configuration. Subcommands: `list`, `set <key> <value>`, `unset <key>`.

## Configuration

Configuration is stored in git config under the `gl-links.*` namespace, and read
with `git config`, so the usual `--local` / `--global` precedence applies.

In effect:

- `gl-links.retention-days` – Days of snapshots `cache prune` keeps (default: 7).
- `gl-links.modification-threshold` – Percentage of a line's content that may
  change before the link is reported as `modified` rather than followed
  (default: 30).

Read but not yet acted on. They parse, `config set` accepts them, and nothing
in the tool consults them — listed here so the gap is visible rather than
discovered:

- `gl-links.rename-threshold` – Intended as the similarity threshold for rename
  detection (default: 50). There is no rename detection yet.
- `gl-links.track-branch` – Branches whose history counts as authoritative
  (multi-valued, default: `main`).
- `gl-links.track-feature-branches` – How many feature branches to track
  (default: 5).

`gl-links.retention-months` was documented and never existed: nothing reads it,
and `cache prune` keeps a flat window of days. Setting it has no effect.

## Files

- `.git/gl-links/` – All state, removed entirely by `cache clear`.
- `.git/gl-links/snapshots/<commit-sha>.json` – One snapshot per recorded commit.
- `.git/gl-links/index.json` – Snapshot index (format version 1).

Nothing is written outside `.git/`, so no state is committed, and a clone starts
with an empty cache that the next run rebuilds from history.

## Exit Status

- `0` – Success. For the default command this includes runs that reported broken
  links: updating what it can and reporting the rest is the intended outcome.
- `1` – An error, an unknown command, or **any broken link found by
  `validate`**. That last one is what makes `validate` usable in a hook or a CI
  job; it exited 0 regardless until it was fixed.

## Examples

Update links in the current repository:

```
git-glfix
```

Validate links without changes:

```
git-glfix validate
```
