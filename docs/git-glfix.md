# git-glfix

Maintain gl: links in a git repository by tracking line changes.

## Synopsis

```
git-glfix [options] [command [args]]
```

## Description

git-glfix tracks and updates gl: links (e.g., `gl:path/to/file#L123`) as files change over time. It records snapshots in the repository and uses git history to adjust line references.

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

- `status` – Show repository and snapshot status.
- `validate` – Scan for broken links without making changes.
- `cache <subcommand>` – Manage the snapshot cache. Subcommands: `list`, `show <commit>`, `clear`, `prune`, `rebuild`.
- `config <subcommand>` – View or modify configuration. Subcommands: `list`, `set <key> <value>`, `unset <key>`.

## Configuration

Configuration is stored in git config under the `gl-links.*` namespace:

- `gl-links.retention-days` – Days to keep daily snapshots (default: 7).
- `gl-links.retention-months` – Months to keep monthly snapshots (default: 1).
- `gl-links.modification-threshold` – Percentage change to trigger warning (default: 30).
- `gl-links.rename-threshold` – Similarity threshold for rename detection (default: 50).

## Files

- `.git/gl-links/` – Snapshot cache and index.

## Exit Status

0 on success; non-zero on error.

## Examples

Update links in the current repository:

```
git-glfix
```

Validate links without changes:

```
git-glfix validate
```
