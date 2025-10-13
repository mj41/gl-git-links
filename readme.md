# gl-git-links repo

Disclaimer: this is a work-in-progress repository for developing the `gl:` git-links specification and related tools. Contents may change frequently. Code was vibe coded and not reviewed.

## gl: specification

The `gl:` URI scheme specification: [docs/spec/gl-spec.md](./docs/spec/gl-spec.md)

## gl: git-links tools

### repo-git-links-fix

Status: planned

When you add a link to a file line like `gl:docs/spec/gl-spec.md#L59`, after some time the referenced file will change. To keep the link valid and pointing to the same content, you can use the repo-git-links-fix tool that will update all links in your repository to point to the correct line numbers. 

How? When a link is added and committed to the main repo branch, it is considered valid. In the meantime, lines are added or removed before the linked line. Since your git history knows when the link was added and when lines were added or removed, you can calculate the correct line number for the link.
