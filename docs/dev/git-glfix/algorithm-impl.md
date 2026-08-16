```markdown
# Algorithm Implementation Details

This document describes the implementation of the line tracking algorithm in `git-glfix`.

## Overview

The current implementation (gl:cmd/git-glfix/main.go#L1) provides an incremental tracking
algorithm with snapshot caching as specified in gl:docs/spec/git-glfix/incremental-tracking.md#L1.

## Main Workflow

The tool executes in four phases (gl:cmd/git-glfix/main.go#L174):

### Phase 1: Link Discovery

Function: `discoverLinks()` (gl:cmd/git-glfix/main.go#L741)

1. Get all git-tracked files via `git ls-files`
2. Filter out binary files using null-byte detection (gl:cmd/git-glfix/main.go#L791)
3. Scan each file for `gl:` pattern with line numbers
4. Resolve relative paths to absolute repository paths (gl:cmd/git-glfix/main.go#L808)

Pattern used: `gl:(\S+?)#L(\d+)` matching the gl: URI scheme (gl:docs/spec/gl-spec.md#L22)

### Phase 2: Snapshot Selection & Incremental Tracking

Function: `selectBestSnapshot()` (gl:cmd/git-glfix/main.go#L500)

The tool selects an existing snapshot to track from (prefers same branch, then ancestor commits).

Function: `trackIncrementally()` (gl:cmd/git-glfix/main.go#L549)

For cached links, track changes commit-by-commit from snapshot to HEAD.

Function: `initializeNewLink()` (gl:cmd/git-glfix/main.go#L916)

For new (undiscovered) links:

1. **Get Origin Commit**: Use `git blame` to find when the link was added (gl:cmd/git-glfix/main.go#L973)
2. **Verify Target Exists**: Check target file exists on filesystem
3. **Track to HEAD**: Use `git blame --reverse` to trace line movement (gl:cmd/git-glfix/main.go#L990)

### Phase 3: Update Application

Function: `applyUpdates()` (gl:cmd/git-glfix/main.go#L1192)

1. Group updates by source file
2. For dry-run: print proposed changes
3. For actual run: read file, replace patterns, write back

### Phase 4: Reporting

Function: `reportResults()` (gl:cmd/git-glfix/main.go#L1289)

Output formats:
- Human-readable summary to stdout
- JSON format for CI integration (gl:cmd/git-glfix/main.go#L1325)

## Git Commands Used

| Command | Purpose | Used In |
|---------|---------|---------|
| `git ls-files` | Get tracked files | `getTrackedFiles()` (gl:cmd/git-glfix/main.go#L328) |
| `git blame -L` | Get origin commit | `getOriginCommit()` (gl:cmd/git-glfix/main.go#L973) |
| `git blame --reverse` | Track line forward | `trackLineToHead()` (gl:cmd/git-glfix/main.go#L990) |
| `git blame HEAD` | Get HEAD line origin | `trackHeadToWorkTree()` (gl:cmd/git-glfix/main.go#L1166) |
| `git blame` (no ref) | Get worktree blame | `trackHeadToWorkTree()` (gl:cmd/git-glfix/main.go#L1166) |
| `git log --reverse` | Find next commit | `recoverLostLine()` (gl:cmd/git-glfix/main.go#L1032) |
| `git diff -U0` | Get line-level diff | `recoverLostLine()` (gl:cmd/git-glfix/main.go#L1087) |

## Heuristic Recovery

When `git blame --reverse` loses track of a line (deleted), the tool attempts heuristic recovery
(gl:cmd/git-glfix/main.go#L1032):

1. Find the commit where the line was last seen
2. Get the diff between that commit and the next
3. Extract deleted and added lines in the same hunk
4. Calculate similarity scores using Levenshtein distance (gl:cmd/git-glfix/main.go#L1411)
5. If best match > 60% similarity, continue tracking from new line

This handles cases like:
- Line reformatting
- Minor content changes
- Line position swaps within a function

Test case: TC-018 (gl:docs/dev/git-glfix/test-cases.md#L130)

## Data Structures

### Snapshot (gl:cmd/git-glfix/main.go#L54)

```go
type Snapshot struct {
    CommitSHA string    // Snapshot commit
    Branch    string    // Branch at time of snapshot
    Timestamp time.Time // When snapshot was created
    Links     []Link    // All tracked links with their state
}
```

### Link (gl:cmd/git-glfix/main.go#L63)

```go
type Link struct {
    SourceFile         string // File containing the gl: link
    SourceLine         int    // 1-indexed line number in source
    TargetFile         string // Normalized absolute path to target
    TargetFileOriginal string // Original path as written (./foo.go)
    TrackedLine        int    // Line number being tracked in target
}
```

### UpdateResult (gl:cmd/git-glfix/main.go#L98)

```go
type UpdateResult struct {
    Link    *Link
    OldLine int    // Original line number in link
    NewLine int    // Computed current line number
    Status  string // "updated", "unchanged", "broken", "error"
    Message string // Error/warning message
}
```

## Limitations vs Full Spec

The current implementation now includes the core incremental tracking algorithm from 
gl:docs/spec/git-glfix/incremental-tracking.md#L1:

| Feature | Spec | Current |
|---------|------|---------|
| Snapshot caching | Yes | Yes |
| Branch tracking | Yes | Yes |
| Rename detection | Yes | No |
| Modification warnings | Yes | Yes (threshold-based) |
| Content hashing | Yes | Yes |
| Retention policies | Yes | Yes (max_snapshots, max_age_days) |

Rename detection is planned for future implementation.

## Testing

The test tool (gl:cmd/git-glfix-test/main.go#L1) validates the algorithm against
the gl-exA test repository. See gl:docs/dev/git-glfix/test-cases.md#L1 for test cases.

```
