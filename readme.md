# gl-git-links repo

Disclaimer: this is a work-in-progress repository for developing the `gl:` git-links specification and related tools. Contents may change frequently. Code was developed rapidly and has not been thoroughly reviewed.

## gl: specification

The `gl:` URI scheme specification: [docs/spec/gl-spec.md](./docs/spec/gl-spec.md)

## gl: git-links tools

### git-glfix

When a link to a file line, such as `gl:docs/spec/gl-spec.md#L59`, is added, the referenced file may change over time. To maintain link validity and ensure it points to the same content, the `git-glfix` tool can be used. This tool updates all links in the repository to point to the correct line numbers.

How? When a link is added and committed to the main repository branch, it is considered valid. Subsequently, lines may be added or removed before the linked line. Since the git history tracks when the link was added and when lines were modified, the correct line number for the link can be calculated.

A link may also address a range, such as `gl:docs/spec/gl-spec.md#L59-L74`. Both ends are tracked independently, so the range moves when lines are inserted above it and widens or narrows when lines are inserted or deleted inside it.

**Command-line tool:**
- Source: [cmd/git-glfix/main.go](cmd/git-glfix/main.go)
- Documentation: [docs/git-glfix.md](docs/git-glfix.md)
- Specification: [docs/spec/git-glfix/](docs/spec/git-glfix/)
- Developer docs: [docs/dev/git-glfix/readme.md](docs/dev/git-glfix/readme.md)

**Go library (`pkg/glcache`):**

For programmatic access to gl: links and cache data:

```go
import "github.com/mj41/gl-git-links/pkg/glcache"

// Discover gl: links (including uncommitted files)
links, _ := glcache.DiscoverLinks(glcache.DiscoverOptions{
    IncludeUntracked: true,
})

// Query backlinks to a documentation file
backlinks, _ := glcache.QueryLinksByTarget("docs/README.md",
    glcache.DiscoverOptions{IncludeUntracked: true})
```

- Documentation: [docs/dev/glcache-library.md](docs/dev/glcache-library.md)

**Use case:** Generate back-reference documentation showing which files link to each documentation page (see [ipm-drawio md-back-refs integration](../ipm-drawio/wip/md-back-refs/spec.md)).

### gl-exA example repository

The [gl-exA](https://github.com/mj41/gl-exA) is example repository where `gl:` links are used and `git-glfix` tool was applied to keep them valid.

## gl: git-links projects

### VS Code extension mj41.gl-git-links

Visual Studio Code extension [mj41.gl-git-links](https://marketplace.visualstudio.com/items?itemName=mj41.gl-git-links) for `gl:` git link syntax. Source code: [mj41/vscode-gl-git-links](https://github.com/mj41/vscode-gl-git-links).
