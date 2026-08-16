# Incremental Link Tracking Algorithm

This document outlines a new algorithm for the `git-glfix` tool, designed to efficiently maintain `gl:` links by tracking line movements incrementally across git commits.

## 1. Core Concept

The algorithm shifts from a stateless "recompute everything" model to a **stateful, incremental** model using **Multi-Level Caching**. Instead of a single state, it maintains multiple snapshots of the link registry at different points in history. This allows the tool to recover efficiently from branch switches, rebases, or history rewrites by picking the nearest valid ancestor snapshot.

## 2. Storage

### 2.1. Location
Snapshots are stored in `.git/gl-links/` directory:
```
.git/gl-links/
├── config.json          # Tool configuration (overrides git config)
├── snapshots/
│   ├── <commit_sha>.json
│   └── ...
└── index.json           # Snapshot registry index
```

This location:
- Is automatically excluded from version control (inside `.git/`)
- Is local to each clone (not shared)
- Survives branch switches and stashes

### 2.2. Format
All data is stored as JSON for simplicity and debuggability.

## 3. Configuration

Configuration uses git config with the `gl-links` section. Repository-level config (`.git/config`) is preferred.

### Git Config Options

```ini
[gl-links]
    # Branches to track (space-separated or multi-value)
    # Snapshots are created per tracked branch
    track-branch = main
    track-branch = develop

    # Number of feature branches to track (most recently modified)
    # These are branches NOT listed in track-branch
    # Set to 0 to disable feature branch tracking
    track-feature-branches = 5

    # Retention: days to keep daily snapshots (default: 7)
    retention-days = 7

    # Retention: months to keep monthly snapshots (default: 1)
    retention-months = 1

    # Modification threshold percentage (default: 30)
    # Changes below this are not reported as modifications
    modification-threshold = 30

    # Rename detection similarity threshold (default: 50)
    rename-threshold = 50
```

### Setting Configuration

```bash
# Track a branch
git config gl-links.track-branch main

# Add another branch (append)
git config --add gl-links.track-branch develop

# Set retention days
git config gl-links.retention-days 14

# List current config
git config --get-regexp gl-links
```

## 4. Data Structures

### 4.1. Snapshot Registry (index.json)
```json
{
  "version": 1,
  "snapshots": [
    {
      "commit_sha": "abc123...",
      "branch": "main",
      "timestamp": "2025-11-30T10:00:00Z",
      "tags": ["head", "daily-2025-11-30"]
    }
  ]
}
```

### 4.2. Snapshot File (<commit_sha>.json)
Represents the state of links at a specific commit.
```json
{
  "commit_sha": "abc123...",
  "branch": "main",
  "timestamp": "2025-11-30T10:00:00Z",
  "links": { ... }
}
```

### 4.3. Link Object
Represents a single `gl:` link instance within a snapshot.
*   **`id`**: Unique identifier (auto-incremented sequence number per snapshot).
*   **`source_file`**: Path to the file containing the link.
*   **`source_line`**: Line number in source file where the link appears.
*   **`target_file`**: Path to the file being referenced (absolute repo path, normalized from relative if needed).
*   **`target_file_original`**: Original path as written in the link (preserved for relative paths like `./file.md`).
*   **`tracked_line`**: The line number in `target_file` at this snapshot's commit.
*   **`line_content_hash`**: MD5 hash of trimmed target line content (for detecting semantic changes).
*   **`history`**: Metadata about origin and modifications.

### 4.4. History Object
Tracks changes to the target line and file.
*   **`origin_commit`**: Commit where the link was introduced.
*   **`origin_line`**: Original line number at `origin_commit`.
*   **`modifications`**: List of `{commit, change_percent}` for significant content changes.
*   **`renames`**: List of `{commit, old_path, new_path}` for target file renames/moves.

## 5. Algorithm Workflow

### Phase 1: Snapshot Selection (The "Time Machine")
1.  **Identify Candidate**: Iterate through stored snapshots for current branch.
2.  **Validate Ancestry**: Find the snapshot $S_{best}$ such that:
    *   $S_{best}.commit\_sha$ is an ancestor of current `HEAD`.
    *   $S_{best}$ is the "youngest" (most recent) among valid ancestors.
3.  **Fallback**:
    *   If no ancestor found (e.g., fresh clone, unrelated history), start with an empty state and scan from the first commit.
    *   If a rebase occurred, the previous "HEAD" snapshot might no longer be an ancestor. The algorithm naturally falls back to an older, stable snapshot.

### Phase 2: Incremental Tracking
1.  **Load State**: Initialize current link states from $S_{best}$.
2.  **Compute Delta**: Identify commits between $S_{best}.commit\_sha$ and `HEAD`.
3.  **Process Commits**: Apply the diff logic sequentially for each commit:
    *   Track line number changes (insertions, deletions).
    *   Detect line content modifications (store change percentage).
    *   Detect target file renames/moves using `git log --follow` or diff rename detection.

### Phase 3: Target File Rename Tracking
When a target file is renamed or moved:
1.  Detect rename via git's rename detection (similarity threshold).
2.  Record in link's `history.renames`: `{commit, old_path, new_path}`.
3.  Update `target_file` in the link object to the new path.
4.  Report rename to user (similar to modification warnings).

Note: The link text in source files is **not** auto-updated for renames. The tool reports renames so the user can decide whether to update the path manually.

### Phase 4: Snapshot Management & Retention
After processing to `HEAD`, the tool saves a new snapshot and prunes old ones.

#### Retention Strategy (Tiered Cache)
1.  **Current HEAD**: Always save the state of the current run.
2.  **Time-Based Tiers**:
    *   **Recent**: Keep 1 snapshot per day for the last N days (configurable via `retention-days`).
    *   **Monthly**: Keep 1 snapshot for the 1st day of each previous month (if `retention-monthly` is true).

#### Pruning
*   Runs automatically after each snapshot save.
*   Removes snapshots that do not fit into the retention tiers.
*   Snapshots are always full (no differential snapshots - KISS principle).

### Phase 5: Application
1.  **Update Source Files**: Update links in the working directory based on the calculated state at `HEAD`.
2.  **Report**:
    *   Broken links (deleted target lines).
    *   Modified target lines (with change percentage).
    *   Renamed/moved target files.

## 6. CLI Commands

### Main Command
```bash
git-glfix [options]
```

#### Options
| Option | Description |
|--------|-------------|
| `--dry-run` | Print what would be changed without modifying files |
| `--verbose` | Show detailed tracking information |
| `--json` | Output in JSON format (for CI/CD and scripting) |
| `--update-modified` | Update links even if they are already modified in working directory |

### Cache Management
```bash
git-glfix cache <subcommand>
```

| Subcommand | Description |
|------------|-------------|
| `list` | List all snapshots with commit, branch, timestamp, and tags |
| `show <commit>` | Show details of a specific snapshot |
| `prune` | Manually trigger snapshot cleanup based on retention policy |
| `clear` | Remove all snapshots (forces full rescan on next run) |
| `rebuild` | Force full rescan and rebuild cache from scratch |

### Configuration
```bash
git-glfix config <subcommand>
```

| Subcommand | Description |
|------------|-------------|
| `list` | Show current configuration (from git config) |
| `set <key> <value>` | Set a configuration value |
| `unset <key>` | Remove a configuration value |

Alternatively, use `git config` directly:
```bash
git config gl-links.<key> <value>
```

### Diagnostics
```bash
git-glfix status
```
Shows:
*   Current branch and HEAD commit.
*   Number of tracked links.
*   Best matching snapshot (if any).
*   Commits since last snapshot.

```bash
git-glfix validate
```
Validates all links without updating:
*   Checks if target files exist.
*   Checks if target lines exist.
*   Reports broken and stale links.

## 7. Error Handling

### Corrupted Cache
If a snapshot file is corrupted or unreadable:
1.  Log a warning.
2.  Skip the corrupted snapshot.
3.  Fall back to the next valid ancestor snapshot.
4.  If no valid snapshots exist, perform full rescan.

### Missing Commits (Force Push)
If a snapshot references a commit that no longer exists:
1.  Mark snapshot as invalid during ancestry check.
2.  Fall back to older snapshot or full rescan.

### Concurrent Access
The tool does not lock the cache. If multiple instances run simultaneously:
*   Each writes its own snapshot (unique commit SHA).
*   Pruning may have race conditions but is idempotent.
*   For CI/CD, run with `--dry-run` to avoid conflicts.

## 8. Link Discovery

### 8.1. Scope
The tool scans all text files tracked by git (`git ls-files`). Binary files are automatically excluded using git's binary detection (files with null bytes or marked as binary in `.gitattributes`).

### 8.2. Pattern Matching
Links are discovered using the `gl:` URI scheme as defined in [gl-spec.md](gl-spec.md). Only links with line number fragments (`#L<number>`) are tracked.

### 8.3. Relative Path Normalization
Relative paths in links (e.g., `gl:./utils.go#L10` or `gl:../common/types.go#L5`) are normalized to absolute repository paths for internal tracking. The original relative format is preserved in `target_file_original` and restored when updating source files.

## 9. Working Directory Handling

The tool tracks line movements based on **committed changes only**. Uncommitted modifications in the working directory are handled as follows:

1.  **Target files**: Uncommitted changes in target files are included when calculating the current line position. This ensures that after running the tool and committing all files, all links will be valid.
2.  **Source files**: Links in uncommitted source file changes are discovered and tracked.
3.  **Staged vs unstaged**: Both staged and unstaged changes are considered part of the "current state".

## 10. Thresholds and Detection

### 10.1. Modification Threshold
A target line is flagged as "modified" when its content changes by more than **30%** (configurable via `modification-threshold`). Changes below this threshold are considered minor edits and not reported.

### 10.2. Rename Detection Threshold
File renames are detected using git's similarity index. Default threshold is **50%** (git's default). Configurable via `rename-threshold`.

### 10.3. Feature Branch Detection
A "feature branch" is any branch that:
*   Is not listed in `track-branch` configuration.
*   Is not a remote tracking branch (`origin/*`).
*   Is not `HEAD` (detached head state).

The `track-feature-branches` setting limits how many feature branches retain snapshots, selecting the most recently modified ones.

## 11. Multiple Links to Same Target

When multiple links point to the same target file and line:
*   Each link is tracked as a separate Link Object (different `id`, `source_file`, `source_line`).
*   All links share the same tracking state for the target.
*   Updates are applied to all source files simultaneously.

## 12. Output Format

### 12.1. Default (Human-Readable)
Text output suitable for terminal display with colors (when supported).

### 12.2. Machine-Readable
Use `--json` flag for JSON output, suitable for CI/CD integration and scripting.

```bash
git-glfix --dry-run --json
```
