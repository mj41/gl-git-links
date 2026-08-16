# Developer Overview: git-glfix

## Source Code

- gl:cmd/git-glfix/main.go#L1 - Main tool implementation
- gl:cmd/git-glfix-test/main.go#L1 - Test runner

## git-glfix-test Tool

The test runner validates `git-glfix` against the `gl-exA` test repository. It walks through each commit, runs the tool in dry-run JSON mode, and compares results against expected outcomes.

### Usage

```bash
git-glfix-test --repo <path> --tool <path> [--verbose] [--stop-on-fail]
git-glfix-test --repo <path> --tool <path> --test-cache [--verbose]
```

### Options

* `--repo <path>`: Path to the test repository (required)
* `--tool <path>`: Path to the `git-glfix` binary (required)
* `--verbose`: Show detailed output for each test
* `--stop-on-fail`: Stop execution on first test failure
* `--test-cache`: Run incremental caching tests instead of algorithm tests

Both suites run from the repository root with `make test`, which builds the two
binaries first. They need `../gl-exA` beside this checkout, and they refuse to
start if it has uncommitted changes: the harness checks out historical commits
and lets git-glfix rewrite files there, so it cannot tell your work from its own.

### Algorithm Tests (default)

Walks through each commit in the test repository, runs the tool, and compares JSON output against expected results defined in `gl-test.json`.

### Cache Tests (`--test-cache`)

Tests the incremental caching behavior:
1. **Snapshot creation** - Verifies running the tool creates a snapshot
2. **Snapshot loading** - Verifies subsequent runs use cached snapshot
3. **Incremental tracking** - Verifies tracking from snapshot to later commits
4. **Cache clear and rebuild** - Verifies cache management commands
5. **Snapshot selection** - Verifies ancestor snapshot selection logic

### Test Specification (gl-test.json)

Each commit in the test repository can include a `gl-test.json` file defining expected outcomes.

#### Simple Format (legacy)

For basic tests that run `--dry-run --json` and compare results:

```json
{
  "description": "Test description",
  "skip": false,
  "skip_reason": "",
  "expect": [
    {
      "file": "path/to/link_file.md",
      "line": 5,
      "old": 10,
      "new": 15,
      "status": "updated"
    }
  ]
}
```

Status values: `updated`, `unchanged`, `broken`

#### Extended Format

For complex tests requiring setup commands, custom arguments, or file content verification:

```json
{
  "description": "Test description",
  "tests": [
    {
      "name": "test name for output",
      "skip": false,
      "setup": ["shell command to run before test"],
      "args": ["--custom-flag"],
      "expect": [{"file": "...", "line": 1, "old": 1, "new": 2, "status": "updated"}],
      "expect_stderr": "substring to match in stderr",
      "expect_files": {"path/to/file.md": "expected content substring"},
      "cleanup": ["shell command to run after test"]
    }
  ]
}
```

Extended test fields:
- `name`: Test name shown in output
- `skip`: Skip this sub-test if true
- `setup`: Shell commands run before the test (e.g., modify files)
- `args`: Additional CLI arguments (default: `--dry-run --json`)
- `expect`: Expected link results (same as simple format)
- `expect_stderr`: Substring that must appear in stderr
- `expect_files`: Map of file paths to expected content substrings
- `cleanup`: Shell commands run after the test (e.g., restore files)

The test runner resets to clean git state before each sub-test.

## Related Dev Documentation

- [Algorithm Implementation](./algorithm-impl.md) - detailed implementation documentation with source references
- [Cache Tracking Example](./cache-tracking-example.md) - step-by-step example of how the incremental cache tracking works
- [Test Cases](./test-cases.md) - test cases for gl-exA test repository

## Related Spec Documentation

- [Overview of git-glfix](../../spec/git-glfix/overview.md) - high-level overview of the tool's purpose and functionality
- [Incremental Tracking](../../spec/git-glfix/incremental-tracking.md) - advanced caching algorithm specification
- [gl-git-links specification](../../spec/gl-spec.md) - user-facing specification of the `gl:` URI scheme

## Key Concepts

| Concept | Spec | Implementation |
|---------|------|----------------|
| Link discovery | gl:docs/spec/git-glfix/overview.md#L26 | gl:cmd/git-glfix/main.go#L860 |
| Origin determination | gl:docs/spec/git-glfix/overview.md#L36 | gl:cmd/git-glfix/main.go#L1159 |
| Line tracking | gl:docs/spec/git-glfix/overview.md#L41 | gl:cmd/git-glfix/main.go#L1176 |
| Incremental tracking | gl:docs/spec/git-glfix/incremental-tracking.md#L1 | gl:cmd/git-glfix/main.go#L652 |
| Snapshot selection | gl:docs/spec/git-glfix/incremental-tracking.md#L100 | gl:cmd/git-glfix/main.go#L603 |
| Path resolution | gl:docs/spec/gl-spec.md#L37 | gl:cmd/git-glfix/main.go#L934 |
