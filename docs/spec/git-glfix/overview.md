# git-glfix tool specification

## 1. Overview

The `git-glfix` tool automates the maintenance of `gl:` links within a git repository. It ensures that links pointing to specific line numbers (e.g., `gl:path/to/file.md#L42`) remain accurate even as the referenced files are modified over time.

## 2. Problem Statement

Documentation and code comments often reference specific lines in other files. As code evolves:
1.  Lines are added or removed in the referenced file.
2.  The content originally at line 42 might move to line 50, or line 30.
3.  Static links become stale, pointing to incorrect or irrelevant content.

Manually updating these links is error-prone and tedious.

## 3. Solution

The `git-glfix` tool uses git history to track the movement of referenced lines. It:
1.  Scans the repository for `gl:` links containing line numbers.
2.  Determines the "validity origin" of each link (when it was added or last updated).
3.  Traces the evolution of the referenced line from that origin to the current `HEAD`.
4.  Updates the link with the new line number.

## 4. Algorithm

### 4.1 Link Discovery

The tool scans all files tracked by git (using `git ls-files`) to ensure `.gitignore` rules are respected. It searches for strings matching the `gl:` URI scheme defined in [gl-spec.md](../gl-spec.md).

Supported link formats include:
*   Absolute paths: `gl:path/to/file.ext#L<number>` (relative to repo root)
*   Relative paths: `gl:./path/to/file.ext#L<number>` or `gl:./../path/to/file.ext#L<number>` (relative to the source file)

The tool resolves relative paths to absolute repository paths before processing. Only links with a specific line number fragment (`#L<number>`) are targeted for updates.

### 4.2 Origin Determination

For each discovered link in `source_file`:

Identify the commit `link_commit_origin` where this specific link string was introduced or last modified in `source_file`.

This can be found using `git blame` on the `source_file`.

### 4.3 Line Tracking

For the `target_file` referenced by the link. Identify the content at `target_line` in `link_commit_origin`. Calculate the new line number `new_target_line` in the current working directory (or `HEAD`) by replaying changes (diffs) affecting `target_file` from `link_commit_origin` to `HEAD`.
* This logic is similar to `git log -L` or how merge tools track lines.
* If lines were added before `target_line`, `new_target_line` increases.
* If lines were removed before `target_line`, `new_target_line` decreases.

### 4.4 Resolution

Moved: If the line content exists at `new_target_line`, update the link in `source_file` to `gl:path/to/target.ext#L<new_target_line>`.

Deleted: If the referenced line was deleted, the tool will:
*   Print a warning to `stderr` indicating the file and line of the broken link.
*   Leave the link unchanged in the source file.

Modified: If the line content changed significantly but is still tracked as the "same" line by git, update the index.

## 5. Usage

The tool is designed to be run from the repository root. By default, it modifies files in-place.
```bash
git-glfix [options]
git-glfix <command> [args]
```

### Options

* `--dry-run`: Print what would be changed without modifying files.
* `--verbose`: Show detailed tracking information.
* `--json`: Output results in machine-readable JSON format.
* `--update-modified`: Update links even if they are already modified in working directory.

### Commands

#### status

Show current repository state and snapshot information:
```bash
git-glfix status
```

Output includes:
- Current branch and HEAD commit
- Active snapshot (if any) and how many commits behind HEAD
- Number of links in worktree

#### validate

Scan for broken links without making changes:
```bash
git-glfix validate
```

Reports links where target file doesn't exist or target line is out of range.

#### cache

Manage the snapshot cache stored in `.git/gl-links/`:

```bash
git-glfix cache list              # List all cached snapshots
git-glfix cache show <commit>     # Show details of a specific snapshot
git-glfix cache clear             # Remove all cached snapshots
git-glfix cache prune             # Remove old snapshots per retention policy
git-glfix cache rebuild           # Rebuild snapshot for current HEAD
```

#### config

View or modify configuration:
```bash
git-glfix config                  # Show current configuration
git-glfix config <key> <value>    # Set a configuration value
```

Configuration keys:
- `max_snapshots`: Maximum number of snapshots to retain (default: 10)
- `max_age_days`: Maximum age of snapshots in days (default: 30)
- `modification_threshold`: Percentage change to trigger "modified" warning (default: 30)

### Example

Before:
File `docs/guide.md`:
```markdown
See the implementation in gl:pkg/main.go#L10
```

Change:
5 lines are inserted at the beginning of `pkg/main.go`.

Run:
```bash
git-glfix
```

After:
File `docs/guide.md`:
```markdown
See the implementation in gl:pkg/main.go#L15
```

## 6. See Also

- [gl: specification](../gl-spec.md) - the `gl:` URI scheme specification
- [Developer Overview: git-glfix](../../dev/git-glfix/readme.md) - detailed developer documentation for the tool
- [Algorithm Implementation](../../dev/git-glfix/algorithm-impl.md) - implementation details with source code references
- gl:cmd/git-glfix/main.go#L1 - source code implementation
