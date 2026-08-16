# Test Cases for gl-exA

This document describes the test cases in the `gl-exA` test repository used to verify the `git-glfix` tool's incremental tracking algorithm.

## Repository State

The `gl-exA` repo has **30 commits** implementing comprehensive test scenarios.

### Target Files

| File | Location | Content |
|------|----------|---------|
| `renamed_target.txt` | `gl-test-data/subdir/` | 9 lines - original test target (renamed/moved from root) |
| `multi_target.txt` | `gl-test-data/` | 26 lines - multi-commit tracking target |
| `large_target.txt` | `gl-test-data/` | 101 lines - performance testing target |

### Link Files

| File | Location | Links |
|------|----------|-------|
| `test_link.md` | `gl-test-data/` | `#L2` (manually updated), `#L6` → `subdir/renamed_target.txt` |
| `test_link2.md` | `gl-test-data/` | `#L4`, `#L6` (broken) → `subdir/renamed_target.txt` |
| `multi_link.md` | `gl-test-data/` | `#L10` → `multi_target.txt` |
| `deep_link.md` | `gl-test-data/nested/` | `#L4` → `../subdir/renamed_target.txt` (relative parent path) |
| `absolute_link.md` | `gl-test-data/` | `#L4` → `subdir/renamed_target.txt`, `#L13` → `multi_target.txt` (absolute paths) |
| `code_with_link.go` | `gl-test-data/` | Multiple links in Go comments → various targets |
| `large_links.md` | `gl-test-data/` | `#L10`, `#L50`, `#L90` → `large_target.txt` |

---

## Test Cases

### TC-001: Repository Initialization
**Purpose**: Initial repository setup.

**State**: Basic `readme.md` created.

---

### TC-002: Add Specification
**Purpose**: Add documentation structure.

**State**: `docs/my-tiny-spec.md` created with initial specification content.

---

### TC-003: Implementation v0.0.1
**Purpose**: Add initial implementation.

**State**: `pkg/tinylang/tinylang.go` created with v0.0.1 implementation.

---

### TC-004: Documentation Changes
**Purpose**: Test that documentation changes can affect line numbers.

**State**: New lines prepended to `docs/my-tiny-spec.md`. This commit demonstrates how doc changes can break gl-git-link line numbers.

---

### TC-005: Spec Update
**Purpose**: Further specification updates.

**State**: Intro section added to specification.

---

### TC-006: Add Test Target
**Purpose**: Create the primary test target file for link tracking tests.

**State**: `gl-test-data/test_target.txt` created with 5 lines:
```
Line 1
Line 2
Line 3
Line 4
Line 5
```

---

### TC-007: Add Test Link
**Purpose**: Create the first link file pointing to the test target.

**State**: `gl-test-data/test_link.md` created with link `gl:./test_target.txt#L3`.

**Expected behavior**: This establishes the baseline for all subsequent tracking tests.

---

### TC-008: Line Insertion Before Target
**Purpose**: Verify line tracking when lines are inserted before the referenced line.

**State**: 2 lines inserted at beginning of target file. Original L3 shifted to L5.

**Expected behavior**: Link updates from `#L3` to `#L5`.

---

### TC-009: Line Insertion After Target
**Purpose**: Verify that insertions after target line don't affect the link.

**State**: 3 lines appended at end of target file.

**Expected behavior**: Link remains at `#L5` (unchanged).

---

### TC-010: Line Deletion Before Target
**Purpose**: Verify line tracking when lines are deleted before the referenced line.

**State**: 1 line deleted before target line. Target shifted from L5 to L4.

**Expected behavior**: Link updates from `#L5` to `#L4`.

---

### TC-011: Minor Target Line Modification
**Purpose**: Verify that small changes to target line content are tracked but not flagged.

**State**: Target line content modified ~20% ("Line 3" → "Line 3 updated").

**Expected behavior**: Link stays at same line number. No modification warning (below 30% threshold).

---

### TC-012: Major Target Line Modification
**Purpose**: Verify that significant content changes trigger a modification warning.

**State**: Target line content modified ~60% (now reads "Completely rewritten content here").

**Expected behavior**: Link stays at same line number. Tool reports modification warning with percentage.

---

### TC-013: Target File Rename
**Purpose**: Verify file rename detection and reporting.

**State**: `test_target.txt` renamed to `renamed_target.txt`.

**Expected behavior**: Tool detects rename, reports to user. Internal tracking updates to new path. Link text NOT auto-updated (user decision).

---

### TC-014: Target File Move to Different Directory
**Purpose**: Verify file move detection (different from simple rename).

**State**: `renamed_target.txt` moved to `gl-test-data/subdir/renamed_target.txt`.

**Expected behavior**: Tool detects move as rename, reports path change to user.

---

### TC-015: Multiple Links to Same Target
**Purpose**: Verify handling of multiple links pointing to same file/line.

**State**:
- `test_link2.md` contains links to `#L4` and `#L6` of same target
- `test_link.md` contains links to `#L2` and `#L6` of same target

**Expected behavior**: Both files tracked separately, updates applied to both.

---

### TC-016: Relative Path Link (Parent Directory)
**Purpose**: Verify relative path handling with `../` notation.

**State**: `gl-test-data/nested/deep_link.md` contains `gl:../subdir/renamed_target.txt#L4`.

**Expected behavior**: Path normalized internally, original relative format preserved on update.

---

### TC-017: Link Update (Re-validated Link)
**Purpose**: Verify that manually updated links reset their origin.

**State**: Link in `test_link.md` manually changed from `#L4` to `#L2`.

**Expected behavior**: New origin commit recorded, tracking starts fresh from new line.

---

### TC-018: Heuristic Recovery - Line Deleted and Similar Added
**Purpose**: Test the heuristic recovery when git loses track of a line.

**State**: Original target line deleted, similar line ("Line 4 reconstructed version") added nearby.

**Expected behavior**: Tool attempts heuristic recovery, reports recovery with confidence score, link updated to new line if successful.

---

### TC-019: Target Line Deleted (Broken Link)
**Purpose**: Verify broken link detection and reporting.

**State**: Target line at L7 deleted without replacement. Link in `test_link2.md` at `#L6` is now broken.

**Expected behavior**: Tool reports broken link, link NOT updated (left as-is), warning to stderr.

---

### TC-020: Multiple Commits Between Runs (Setup)
**Purpose**: Verify incremental tracking across multiple commits.

**State**: `multi_target.txt` created with 20 lines. `multi_link.md` has link to L10.

---

### TC-021: Multi-Commit Insert
**Purpose**: Continued multi-commit tracking.

**State**: 5 lines inserted at L5 in `multi_target.txt`. Target shifted from L10 to L15.

---

### TC-022: Multi-Commit Delete
**Purpose**: Continued multi-commit tracking.

**State**: 2 lines deleted at beginning of `multi_target.txt`. Target shifted from L15 to L13.

---

### TC-023: Multi-Commit Modify
**Purpose**: Continued multi-commit tracking.

**State**: Target line at current position (L10 in final state) slightly modified (now reads "Multi Line 10 - TARGET (verified)").

**Expected behavior**: After all commits, tool correctly calculates final position, tracks through all intermediate commits.

---

### TC-024: Absolute Path Link
**Purpose**: Verify absolute path links work correctly.

**State**: `absolute_link.md` contains:
- `gl:gl-test-data/subdir/renamed_target.txt#L4`
- `gl:gl-test-data/multi_target.txt#L13`

**Expected behavior**: Path stored as-is (no `./` prefix), tracking works same as relative.

---

### TC-025: Link in Non-Markdown File
**Purpose**: Verify links discovered in various file types.

**State**: `code_with_link.go` contains `gl:` links in Go comments.

**Expected behavior**: Links discovered and tracked, file type doesn't affect behavior.

---

### TC-026: Empty Lines and Whitespace
**Purpose**: Verify handling of whitespace-only changes.

**State**: Empty lines added/removed around targets, trailing whitespace changes applied.

**Expected behavior**: Empty line changes shift line numbers correctly, trailing whitespace change is minor modification (< 30%).

---

### TC-027: Large File Setup
**Purpose**: Verify performance with larger files.

**State**: `large_target.txt` with 100 lines. `large_links.md` has links to L10, L50, L90.

---

### TC-028: Large File Changes
**Purpose**: Performance testing with multiple insertions/deletions.

**State**:
- 5 lines inserted at position 5
- 3 lines inserted at position 70
- Lines 020-022 deleted

**Expected behavior**: All links tracked correctly. Performance baseline established.

---

### TC-029: Modified Source File Setup
**Purpose**: Setup for testing `--update-modified` flag behavior.

**State**: `modify_source_link.md` with link to `modify_source_target.txt#L3`.

---

### TC-030: Shift for Modified Source Test
**Purpose**: Verify `--update-modified` flag behavior.

**State**: 2 lines inserted before target, shifting L3 to L5.

**Expected behavior**:
- Without `--update-modified`: If source file has uncommitted changes, skip with warning.
- With `--update-modified`: Update the link even if source file is modified.

---

## Commit Sequence Summary

| Commit | Description | Key Test |
|--------|-------------|----------|
| 001 | Repository initialization | Setup |
| 002 | Add specification | Documentation |
| 003 | Implement v0.0.1 | Code structure |
| 004 | Documentation changes | Line number shifts |
| 005 | Spec update with intro | Doc updates |
| 006 | Add test target | 5-line target file |
| 007 | Add test link | Initial link to L3 |
| 008 | Insert 2 lines before target | Line shift (+2): L3→L5 |
| 009 | Insert 3 lines after target | No change to link |
| 010 | Delete 1 line before target | Line shift (-1): L5→L4 |
| 011 | Minor target modification (~20%) | Below threshold |
| 012 | Major target modification (~60%) | Above threshold |
| 013 | Rename target file | Rename detection |
| 014 | Move to subdirectory | Path change |
| 015 | Add second link file | Multiple links |
| 016 | Add nested relative link | Path normalization (../) |
| 017 | Manual link update | Origin reset |
| 018 | Delete + similar add | Heuristic recovery |
| 019 | Delete target line | Broken link |
| 020 | Multi-commit target setup | New target + link at L10 |
| 021 | Insert 5 lines at L5 | L10→L15 |
| 022 | Delete 2 lines at beginning | L15→L13 |
| 023 | Minor modification | Track through changes |
| 024 | Absolute path link | Path format |
| 025 | Link in .go file | File type support |
| 026 | Whitespace changes | Edge case |
| 027 | Large file setup | 100 lines, links at L10/L50/L90 |
| 028 | Large file changes | Performance test |
| 029 | Add modified source test link | Setup for --update-modified |
| 030 | Shift for modified test | --update-modified behavior |

## Implementation Notes

1. Each test case is a separate commit in `gl-exA-src/assets/`
2. Tests are sequential - each builds on previous state
3. Branch divergence test (TC-021 original) deferred - requires branch support in `git-rgen-tool`
4. File deletions use `.rgen-delete` marker file
5. Rename detection relies on git's similarity threshold

## Generating the Test Repository

The `gl-exA` test repository is generated from `gl-exA-src` assets using the [git-rgen-tool](https://github.com/mj41/git-rgen-tool).

```bash
# From git-rgen-tool directory
rm -rf ../gl-exA
go run ./cmd/rgen --conf ../gl-exA-src/rgen-conf.json
```

This creates the full git history in `gl-exA/` based on the commit assets in `gl-exA-src/assets/`.

See [git-rgen-tool/docs/rgen-spec.md](https://github.com/mj41/git-rgen-tool/blob/main/docs/rgen-spec.md) for the specification of asset structure.

## Running Tests

Build and run the test suite using Makefile targets:

```bash
cd gl-git-links

# Build and run tests
make test

# Or run with verbose output
make build
./git-glfix-test -repo ../gl-exA -tool ./git-glfix -verbose
```

Available Makefile targets:
- `make build` - Build `git-glfix` binary
- `make test` - Build and run tests against `gl-exA`
- `make regen-exa` - Regenerate `gl-exA` test repository from `gl-exA-src` assets
- `make clean` - Remove built binaries

The test runner walks through each commit in `gl-exA`, runs `git-glfix --dry-run --json`, and compares results against expected outcomes in `gl-test.json` files.

See [git-glfix-test documentation](./readme.md#git-glfix-test-tool) for CLI options.
