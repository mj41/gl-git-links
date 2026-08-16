# glcache Library

Package `glcache` provides a Go library for working with `gl:` link caches in git repositories.

**Repository:** `github.com/mj41/gl-git-links/pkg/glcache`

## Related Documentation

- [git-glfix Tool](../git-glfix.md) - Command-line tool for maintaining gl: links
- [gl: Specification](../spec/gl-spec.md) - URI scheme specification
- [Incremental Tracking](../spec/git-glfix/incremental-tracking.md) - Algorithm details
- [Cache Format](../spec/git-glfix/incremental-tracking.md#L17) - Storage structure

## Features

- Load and save snapshots of gl: link states at specific commits
- Discover gl: links in tracked or untracked files
- Query links by target file (useful for generating back-references)
- Access git repository information

## Usage

### Load Latest Snapshot

```go
import "github.com/mj41/gl-git-links/pkg/glcache"

snapshot, err := glcache.LoadLatestSnapshot()
if err != nil {
    // No snapshot found or error
}
```

### Discover Links

```go
// Discover links in tracked files only
links, err := glcache.DiscoverLinks(glcache.DiscoverOptions{
    IncludeUntracked: false,
})

// Discover links including untracked files
links, err := glcache.DiscoverLinks(glcache.DiscoverOptions{
    IncludeUntracked: true,
})
```

### Query Links by Target (for Back-References)

```go
// Find all links pointing to a specific file
links, err := glcache.QueryLinksByTarget("docs/dev/layout.md", glcache.DiscoverOptions{
    IncludeUntracked: true,
})

for _, link := range links {
    fmt.Printf("%s:%d -> %s#L%d\n",
        link.SourceFile, link.SourceLine,
        link.TargetFile, link.TrackedLine)
}
```

### Save Snapshot

```go
head, _ := glcache.GetHeadCommit()
branch, _ := glcache.GetCurrentBranch()

err := glcache.SaveSnapshot(head, branch, links)
```

### Git Information

```go
head, err := glcache.GetHeadCommit()
branch, err := glcache.GetCurrentBranch()
root, err := glcache.GetRepoRoot()
isAnc := glcache.IsAncestor(commit1, commit2)
```

## Storage

Cache is stored in `.git/gl-links/`:
- `index.json` - Registry of all snapshots
- `snapshots/<commit-sha>.json` - Full snapshot for each commit

## For md-back-refs Integration

To use gl: links as back-references in markdown documentation:

```go
// Find all links pointing to your docs
links, err := glcache.QueryLinksByTarget("docs/dev/layout-gen/layout-alg.md",
    glcache.DiscoverOptions{IncludeUntracked: true})

// Group by target line for section-level back-refs
byLine := make(map[int][]glcache.Link)
for _, link := range links {
    byLine[link.TrackedLine] = append(byLine[link.TrackedLine], link)
}
```

**Live vs Cached Discovery:**
- **Live discovery** (`DiscoverLinks`) includes uncommitted files - best for documentation generation
- **Cached snapshots** (`LoadLatestSnapshot`) faster but only committed state - best for CI/reporting

## Example Code

See working example at `ipm-drawio/wip/glexper/cache-demo.go`:

```bash
cd /path/to/ipm-drawio
./wip/glexper/cache-demo
```

Demonstrates:
- Loading cached snapshot with 221 gl: links
- Querying 68 backlinks to a specific documentation file
- Grouping links by target file and line

## API Reference

### Types

```go
type Link struct {
    ID                 int
    SourceFile         string  // File containing the gl: link
    SourceLine         int     // Line number in source
    TargetFile         string  // File being referenced
    TargetFileOriginal string  // Original path as written
    TrackedLine        int     // Line number in target (start, for a range)
    TrackedEndLine     int     // End line for a range link; 0 for a single line
    LineContentHash    string  // MD5 of line content
    History            History // Origin and modifications
}

type DiscoverOptions struct {
    IncludeUntracked bool // Include untracked/uncommitted files
}
```

### Link grammar

```go
var LinkRe = regexp.MustCompile(`gl:(\S+?)#L([1-9][0-9]*)(?:-L([1-9][0-9]*))?`)
```

`LinkRe` is exported so this module has exactly one definition of what a link
is. `cmd/git-glfix` used to keep a second copy, and they had drifted: the tool
grew range support, this package never did, so the same text parsed differently
depending on which entry point read it — a range came back through
`DiscoverLinks` as a single-line link to its first line, with nothing to signal
the loss.

Groups are path, start line, end line (empty for a single line). The grammar is
the specification's: the first digit is non-zero, so `#L0` and `#L007` are not
line links and are not returned. A link with no line spec is not returned
either — there is no line to track.

### Functions

**Cache Operations:**
- `LoadSnapshot(commitSHA) (*Snapshot, error)`
- `LoadLatestSnapshot() (*Snapshot, error)`
- `SaveSnapshot(commitSHA, branch, links) error`
- `LoadIndex() (*SnapshotIndex, error)`
- `ClearCache() error`

**Link Discovery:**
- `DiscoverLinks(opts) ([]Link, error)`
- `QueryLinksByTarget(targetFile, opts) ([]Link, error)`
- `GroupLinksByTarget(links) map[string][]Link`
- `GroupLinksByTargetLine(links) map[string]map[int][]Link`

**Git Helpers:**
- `GetHeadCommit() (string, error)`
- `GetCurrentBranch() (string, error)`
- `GetRepoRoot() (string, error)`
- `IsAncestor(ancestor, descendant) bool`

**Utilities:**
- `ComputeLineHash(content) string`
