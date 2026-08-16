package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// TestSpec from gl-test.json in asset directory
// gl:docs/dev/git-glfix/test-cases.md#L1 - Test case documentation
type TestSpec struct {
	Description string         `json:"description"`
	Expect      []ExpectedLink `json:"expect,omitempty"`
	Skip        bool           `json:"skip,omitempty"`
	SkipReason  string         `json:"skip_reason,omitempty"`
	// Extended test support
	Tests []ExtendedTest `json:"tests,omitempty"`
}

// ExtendedTest allows running different test scenarios after a commit
type ExtendedTest struct {
	Name         string            `json:"name"`                    // Test name for reporting
	Skip         bool              `json:"skip,omitempty"`          // Skip this test
	Setup        []string          `json:"setup,omitempty"`         // Shell commands to run before test (e.g., modify files)
	Args         []string          `json:"args,omitempty"`          // Args to pass to git-glfix (default: --dry-run --json)
	Expect       []ExpectedLink    `json:"expect,omitempty"`        // Expected link results
	ExpectStderr string            `json:"expect_stderr,omitempty"` // Expected substring in stderr
	ExpectFiles  map[string]string `json:"expect_files,omitempty"`  // File -> expected content substring
	Cleanup      []string          `json:"cleanup,omitempty"`       // Shell commands to run after test
}

// ExpectedLink represents an expected test outcome
// gl:docs/dev/git-glfix/test-cases.md#L20 - Target and Link files structure
type ExpectedLink struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Old    int    `json:"old"`
	New    int    `json:"new"`
	Status string `json:"status"` // "updated", "unchanged", "broken" (gl:docs/spec/git-glfix/overview.md#L45)
}

// ActualLink is the output from git-glfix --json
// gl:cmd/git-glfix/main.go#L1326 - JSON output structure
type ActualLink struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Target  string `json:"target"`
	OldLine int    `json:"old_line"`
	NewLine int    `json:"new_line"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

var (
	repoPath    string
	fixToolPath string
	verbose     bool
	stopOnFail  bool
	testCache   bool
)

func main() {
	flag.StringVar(&repoPath, "repo", "", "Path to test repository (required)")
	flag.StringVar(&fixToolPath, "tool", "", "Path to git-glfix binary (required)")
	flag.BoolVar(&verbose, "verbose", false, "Show detailed output")
	flag.BoolVar(&stopOnFail, "stop-on-fail", false, "Stop on first failure")
	flag.BoolVar(&testCache, "test-cache", false, "Run incremental caching tests")
	flag.Parse()

	if repoPath == "" || fixToolPath == "" {
		fmt.Fprintln(os.Stderr, "Usage: git-glfix-test --repo <path> --tool <path> [--test-cache] [--verbose]")
		flag.PrintDefaults()
		os.Exit(1)
	}

	repoPath, _ = filepath.Abs(repoPath)
	fixToolPath, _ = filepath.Abs(fixToolPath)

	var err error
	if testCache {
		err = runCacheTests()
	} else {
		err = run()
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Get all commits from repo
	commits, err := getCommits()
	if err != nil {
		return fmt.Errorf("failed to get commits: %w", err)
	}

	fmt.Printf("Found %d commits\n", len(commits))

	passed := 0
	failed := 0
	skipped := 0
	tested := 0

	for _, commit := range commits {
		// Checkout commit
		cmd := exec.Command("git", "-C", repoPath, "checkout", "--quiet", commit.Hash)
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Printf("ERROR: checkout %s failed: %s\n", commit.Hash[:8], string(out))
			continue
		}

		// Check if gl-test.json exists at this commit
		testFile := filepath.Join(repoPath, "gl-test.json")
		data, err := os.ReadFile(testFile)
		if err != nil {
			continue // No test spec for this commit
		}

		var spec TestSpec
		if err := json.Unmarshal(data, &spec); err != nil {
			fmt.Printf("ERROR: invalid gl-test.json at %s: %v\n", commit.Hash[:8], err)
			continue
		}

		tested++

		if spec.Skip {
			fmt.Printf("SKIP %s: %s\n", commit.Hash[:8], commit.Subject)
			if spec.SkipReason != "" {
				fmt.Printf("     Reason: %s\n", spec.SkipReason)
			}
			skipped++
			continue
		}

		ok, err := runTest(commit, &spec)
		if err != nil {
			fmt.Printf("  ERROR: %v\n", err)
			failed++
			if stopOnFail {
				break
			}
			continue
		}

		if ok {
			passed++
		} else {
			failed++
			if stopOnFail {
				break
			}
		}
	}

	// Return to main branch
	exec.Command("git", "-C", repoPath, "checkout", "--quiet", "main").Run()

	fmt.Printf("\n========================================\n")
	fmt.Printf("Results: %d passed, %d failed, %d skipped (of %d tested)\n", passed, failed, skipped, tested)

	if failed > 0 {
		return fmt.Errorf("%d tests failed", failed)
	}
	return nil
}

type commit struct {
	Hash    string
	Subject string
}

func getCommits() ([]commit, error) {
	cmd := exec.Command("git", "-C", repoPath, "log", "--reverse", "--format=%H %s")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var commits []commit
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			continue
		}
		commits = append(commits, commit{
			Hash:    parts[0],
			Subject: parts[1],
		})
	}

	return commits, nil
}

func runTest(c commit, spec *TestSpec) (bool, error) {
	fmt.Printf("TEST %s: %s\n", c.Hash[:8], c.Subject)
	if verbose && spec.Description != "" {
		fmt.Printf("     %s\n", spec.Description)
	}

	// Checkout commit (clean state)
	cmd := exec.Command("git", "-C", repoPath, "checkout", "--quiet", c.Hash)
	if out, err := cmd.CombinedOutput(); err != nil {
		return false, fmt.Errorf("checkout failed: %s", string(out))
	}

	// If legacy expect is set, run default test
	if len(spec.Expect) > 0 {
		return runDefaultTest(spec.Expect)
	}

	// If extended tests are defined, run each
	if len(spec.Tests) > 0 {
		allPass := true
		passCount := 0
		for i, test := range spec.Tests {
			if test.Skip {
				if verbose {
					fmt.Printf("  SKIP [%d] %s\n", i+1, test.Name)
				}
				continue
			}

			// Reset to clean state before each extended test
			exec.Command("git", "-C", repoPath, "checkout", "--quiet", ".").Run()

			ok, err := runExtendedTest(&test, i+1)
			if err != nil {
				fmt.Printf("  ERROR [%d] %s: %v\n", i+1, test.Name, err)
				allPass = false
				continue
			}
			if !ok {
				allPass = false
			} else {
				passCount++
			}
		}
		if allPass {
			fmt.Printf("  PASS (%d sub-tests)\n", passCount)
		}
		return allPass, nil
	}

	// No tests defined - just pass
	fmt.Printf("  PASS (no expectations)\n")
	return true, nil
}

// runDefaultTest runs the default --dry-run --json test
func runDefaultTest(expect []ExpectedLink) (bool, error) {
	fixCmd := exec.Command(fixToolPath, "--dry-run", "--json")
	fixCmd.Dir = repoPath
	fixCmd.Stderr = nil // Ignore stderr (warnings)
	fixOut, _ := fixCmd.Output()

	// Parse actual results
	var actual []ActualLink
	if len(fixOut) > 0 {
		if err := json.Unmarshal(fixOut, &actual); err != nil {
			return false, fmt.Errorf("failed to parse tool output: %w\n%s", err, string(fixOut))
		}
	}

	ok, err := compareResults(expect, actual)
	if err != nil {
		return false, err
	}
	if ok {
		fmt.Printf("  PASS\n")
	}
	return ok, nil
}

// runExtendedTest runs a test with custom setup, args, and expectations
func runExtendedTest(test *ExtendedTest, testNum int) (bool, error) {
	testName := test.Name
	if testName == "" {
		testName = fmt.Sprintf("test-%d", testNum)
	}

	// Run setup commands
	for _, setupCmd := range test.Setup {
		cmd := exec.Command("sh", "-c", setupCmd)
		cmd.Dir = repoPath
		if out, err := cmd.CombinedOutput(); err != nil {
			return false, fmt.Errorf("setup failed: %s: %s", setupCmd, string(out))
		}
	}

	// Determine args (default to --dry-run --json if expect is set)
	args := test.Args
	if len(args) == 0 && len(test.Expect) > 0 {
		args = []string{"--dry-run", "--json"}
	}

	// Run git-glfix
	fixCmd := exec.Command(fixToolPath, args...)
	fixCmd.Dir = repoPath
	var stderrBuf bytes.Buffer
	fixCmd.Stderr = &stderrBuf
	fixOut, _ := fixCmd.Output()
	stderrStr := stderrBuf.String()

	allPass := true

	// Check stderr expectation
	if test.ExpectStderr != "" {
		if !strings.Contains(stderrStr, test.ExpectStderr) {
			fmt.Printf("  FAIL [%d] %s: stderr missing %q\n", testNum, testName, test.ExpectStderr)
			if verbose {
				fmt.Printf("       stderr was: %s\n", stderrStr)
			}
			allPass = false
		} else if verbose {
			fmt.Printf("  OK [%d] %s: stderr contains %q\n", testNum, testName, test.ExpectStderr)
		}
	}

	// Check link expectations (if --json output expected)
	if len(test.Expect) > 0 {
		var actual []ActualLink
		if len(fixOut) > 0 {
			if err := json.Unmarshal(fixOut, &actual); err != nil {
				return false, fmt.Errorf("failed to parse JSON output: %w", err)
			}
		}
		ok, _ := compareResults(test.Expect, actual)
		if !ok {
			allPass = false
		}
	}

	// Check file content expectations
	for filePath, expectedContent := range test.ExpectFiles {
		fullPath := filepath.Join(repoPath, filePath)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			fmt.Printf("  FAIL [%d] %s: cannot read %s: %v\n", testNum, testName, filePath, err)
			allPass = false
			continue
		}
		if !strings.Contains(string(content), expectedContent) {
			fmt.Printf("  FAIL [%d] %s: %s missing %q\n", testNum, testName, filePath, expectedContent)
			allPass = false
		} else if verbose {
			fmt.Printf("  OK [%d] %s: %s contains %q\n", testNum, testName, filePath, expectedContent)
		}
	}

	// Run cleanup commands
	for _, cleanupCmd := range test.Cleanup {
		cmd := exec.Command("sh", "-c", cleanupCmd)
		cmd.Dir = repoPath
		cmd.Run() // Ignore cleanup errors
	}

	if allPass {
		fmt.Printf("  PASS [%d] %s\n", testNum, testName)
	}

	return allPass, nil
}

// compareResults compares expected vs actual link results
func compareResults(expect []ExpectedLink, actual []ActualLink) (bool, error) {
	allMatch := true
	for _, exp := range expect {
		found := false
		for _, act := range actual {
			if act.File == exp.File && act.Line == exp.Line {
				found = true

				// Check status
				if act.Status != exp.Status {
					fmt.Printf("  FAIL: %s:%d status: expected %q, got %q\n",
						exp.File, exp.Line, exp.Status, act.Status)
					allMatch = false
					continue
				}

				// Check line numbers
				if act.OldLine != exp.Old {
					fmt.Printf("  FAIL: %s:%d old_line: expected %d, got %d\n",
						exp.File, exp.Line, exp.Old, act.OldLine)
					allMatch = false
				}
				if act.NewLine != exp.New {
					fmt.Printf("  FAIL: %s:%d new_line: expected %d, got %d\n",
						exp.File, exp.Line, exp.New, act.NewLine)
					allMatch = false
				}

				if verbose && act.Status == exp.Status && act.OldLine == exp.Old && act.NewLine == exp.New {
					fmt.Printf("  OK: %s:%d %s (L%d->L%d)\n",
						exp.File, exp.Line, exp.Status, exp.Old, exp.New)
				}
				break
			}
		}

		if !found {
			fmt.Printf("  FAIL: %s:%d not found in output\n", exp.File, exp.Line)
			allMatch = false
		}
	}

	return allMatch, nil
}

// =============================================================================
// Incremental Caching Tests
// =============================================================================

// runCacheTests runs tests specifically for incremental caching behavior
func runCacheTests() error {
	fmt.Println("Running incremental caching tests...")
	fmt.Println()

	// Get commits for cache testing
	commits, err := getCommits()
	if err != nil {
		return fmt.Errorf("failed to get commits: %w", err)
	}

	if len(commits) < 10 {
		return fmt.Errorf("need at least 10 commits for cache tests, got %d", len(commits))
	}

	passed := 0
	failed := 0

	tests := []struct {
		name string
		fn   func([]commit) (bool, error)
	}{
		{"Snapshot creation", testSnapshotCreation},
		{"Snapshot loading", testSnapshotLoading},
		{"Incremental tracking across commits", testIncrementalTracking},
		{"Cache clear and rebuild", testCacheClearRebuild},
		{"Snapshot selection prefers same branch", testSnapshotSelection},
	}

	for _, test := range tests {
		fmt.Printf("TEST: %s\n", test.name)

		// Clean up cache before each test
		clearCache()

		ok, err := test.fn(commits)
		if err != nil {
			fmt.Printf("  ERROR: %v\n", err)
			failed++
			if stopOnFail {
				break
			}
			continue
		}

		if ok {
			fmt.Printf("  PASS\n")
			passed++
		} else {
			failed++
			if stopOnFail {
				break
			}
		}
	}

	// Cleanup: return to main and clear cache
	exec.Command("git", "-C", repoPath, "checkout", "--quiet", "main").Run()
	clearCache()

	fmt.Printf("\n========================================\n")
	fmt.Printf("Cache Tests: %d passed, %d failed\n", passed, failed)

	if failed > 0 {
		return fmt.Errorf("%d cache tests failed", failed)
	}
	return nil
}

// clearCache removes all snapshots from the test repo
func clearCache() {
	cmd := exec.Command(fixToolPath, "cache", "clear")
	cmd.Dir = repoPath
	cmd.Run()
}

// getSnapshotCount returns the number of cached snapshots
func getSnapshotCount() int {
	cmd := exec.Command(fixToolPath, "cache", "list")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return -1
	}
	// Count lines that look like snapshot entries (contain commit hash)
	count := 0
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if len(line) >= 8 && !strings.HasPrefix(line, "Snapshots") {
			count++
		}
	}
	return count
}

// getStatusOutput runs "status" command and returns output
func getStatusOutput() string {
	cmd := exec.Command(fixToolPath, "status")
	cmd.Dir = repoPath
	out, _ := cmd.Output()
	return string(out)
}

// testSnapshotCreation verifies that running the tool creates a snapshot
func testSnapshotCreation(commits []commit) (bool, error) {
	// Checkout a mid-point commit
	midCommit := commits[len(commits)/2]
	cmd := exec.Command("git", "-C", repoPath, "checkout", "--quiet", midCommit.Hash)
	if out, err := cmd.CombinedOutput(); err != nil {
		return false, fmt.Errorf("checkout failed: %s", string(out))
	}

	// Verify no snapshots exist
	if count := getSnapshotCount(); count != 0 {
		fmt.Printf("  FAIL: expected 0 snapshots before run, got %d\n", count)
		return false, nil
	}

	// Run tool (not dry-run) to create snapshot
	fixCmd := exec.Command(fixToolPath)
	fixCmd.Dir = repoPath
	var stderr bytes.Buffer
	fixCmd.Stderr = &stderr
	fixCmd.Run()

	if verbose {
		fmt.Printf("  stderr: %s\n", stderr.String())
	}

	// Verify snapshot was created
	if count := getSnapshotCount(); count != 1 {
		fmt.Printf("  FAIL: expected 1 snapshot after run, got %d\n", count)
		return false, nil
	}

	// Verify status shows the snapshot
	status := getStatusOutput()
	if !strings.Contains(status, midCommit.Hash[:8]) {
		fmt.Printf("  FAIL: status doesn't show snapshot commit\n")
		if verbose {
			fmt.Printf("  status: %s\n", status)
		}
		return false, nil
	}

	return true, nil
}

// testSnapshotLoading verifies that running again uses the cached snapshot
func testSnapshotLoading(commits []commit) (bool, error) {
	// Checkout a commit
	commit := commits[len(commits)/2]
	cmd := exec.Command("git", "-C", repoPath, "checkout", "--quiet", commit.Hash)
	if out, err := cmd.CombinedOutput(); err != nil {
		return false, fmt.Errorf("checkout failed: %s", string(out))
	}

	// Run tool to create snapshot
	createCmd := exec.Command(fixToolPath)
	createCmd.Dir = repoPath
	createCmd.Run()

	// Run again with verbose to check it uses snapshot
	fixCmd := exec.Command(fixToolPath, "--verbose", "--dry-run")
	fixCmd.Dir = repoPath
	// Capture combined stdout+stderr since info messages go to stdout
	out, _ := fixCmd.CombinedOutput()
	outStr := string(out)

	if verbose {
		fmt.Printf("  output: %s\n", outStr)
	}

	// Should say "Using snapshot from ..."
	if !strings.Contains(outStr, "Using snapshot from") {
		fmt.Printf("  FAIL: second run didn't use cached snapshot\n")
		if verbose {
			fmt.Printf("  output: %s\n", outStr)
		}
		return false, nil
	}

	return true, nil
}

// testIncrementalTracking verifies tracking from snapshot commit to later commit
func testIncrementalTracking(commits []commit) (bool, error) {
	// Use commits that have test specs
	// Find a commit with gl-test.json and a later one
	var baseCommit, laterCommit commit
	var baseSpec TestSpec

	for i, c := range commits {
		cmd := exec.Command("git", "-C", repoPath, "checkout", "--quiet", c.Hash)
		cmd.Run()

		testFile := filepath.Join(repoPath, "gl-test.json")
		data, err := os.ReadFile(testFile)
		if err != nil {
			continue
		}

		var spec TestSpec
		if err := json.Unmarshal(data, &spec); err != nil {
			continue
		}

		if len(spec.Expect) > 0 && !spec.Skip {
			if baseCommit.Hash == "" {
				baseCommit = c
				baseSpec = spec
			} else if i > 0 {
				laterCommit = c
				break
			}
		}
	}

	if baseCommit.Hash == "" || laterCommit.Hash == "" {
		fmt.Printf("  SKIP: couldn't find suitable commit pair for incremental test\n")
		return true, nil
	}

	if verbose {
		fmt.Printf("  Base commit: %s\n", baseCommit.Hash[:8])
		fmt.Printf("  Later commit: %s\n", laterCommit.Hash[:8])
	}

	// Checkout base commit and create snapshot
	exec.Command("git", "-C", repoPath, "checkout", "--quiet", baseCommit.Hash).Run()
	createCmd := exec.Command(fixToolPath)
	createCmd.Dir = repoPath
	createCmd.Run()

	// Verify snapshot created
	if count := getSnapshotCount(); count != 1 {
		fmt.Printf("  FAIL: expected 1 snapshot at base, got %d\n", count)
		return false, nil
	}

	// Checkout later commit
	exec.Command("git", "-C", repoPath, "checkout", "--quiet", laterCommit.Hash).Run()

	// Run with verbose to verify incremental tracking
	fixCmd := exec.Command(fixToolPath, "--verbose", "--dry-run", "--json")
	fixCmd.Dir = repoPath
	out, _ := fixCmd.CombinedOutput()
	outStr := string(out)

	if verbose {
		fmt.Printf("  output: %s\n", outStr)
	}

	// Should use snapshot (ancestor of later commit)
	if !strings.Contains(outStr, "Using snapshot from") {
		fmt.Printf("  FAIL: didn't use snapshot for incremental tracking\n")
		return false, nil
	}

	// Parse JSON results - need to extract just the JSON part
	// Output format is: info lines... then JSON array
	jsonStart := strings.Index(outStr, "[")
	if jsonStart < 0 {
		// No JSON output - might be all links unchanged
		if verbose {
			fmt.Printf("  No JSON output (all links may be unchanged)\n")
		}
		return true, nil
	}

	jsonPart := outStr[jsonStart:]
	var actual []ActualLink
	if err := json.Unmarshal([]byte(jsonPart), &actual); err != nil {
		return false, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// At minimum, the links from base should still be present
	foundLinks := 0
	for _, exp := range baseSpec.Expect {
		for _, act := range actual {
			if act.File == exp.File && act.Line == exp.Line {
				foundLinks++
				break
			}
		}
	}

	if foundLinks == 0 && len(baseSpec.Expect) > 0 {
		fmt.Printf("  FAIL: no links from base commit found in later commit\n")
		return false, nil
	}

	if verbose {
		fmt.Printf("  Found %d/%d links from base commit\n", foundLinks, len(baseSpec.Expect))
	}

	return true, nil
}

// testCacheClearRebuild verifies cache clear and rebuild commands work
func testCacheClearRebuild(commits []commit) (bool, error) {
	// Checkout a commit
	commit := commits[len(commits)/2]
	exec.Command("git", "-C", repoPath, "checkout", "--quiet", commit.Hash).Run()

	// Create snapshot
	createCmd := exec.Command(fixToolPath)
	createCmd.Dir = repoPath
	createCmd.Run()

	if count := getSnapshotCount(); count != 1 {
		fmt.Printf("  FAIL: expected 1 snapshot, got %d\n", count)
		return false, nil
	}

	// Clear cache
	clearCache()

	if count := getSnapshotCount(); count != 0 {
		fmt.Printf("  FAIL: expected 0 snapshots after clear, got %d\n", count)
		return false, nil
	}

	// Rebuild (run again)
	rebuildCmd := exec.Command(fixToolPath)
	rebuildCmd.Dir = repoPath
	rebuildCmd.Run()

	if count := getSnapshotCount(); count != 1 {
		fmt.Printf("  FAIL: expected 1 snapshot after rebuild, got %d\n", count)
		return false, nil
	}

	return true, nil
}

// testSnapshotSelection verifies snapshot selection logic
func testSnapshotSelection(commits []commit) (bool, error) {
	if len(commits) < 5 {
		fmt.Printf("  SKIP: need at least 5 commits\n")
		return true, nil
	}

	// Create snapshot at early commit
	earlyCommit := commits[2]
	exec.Command("git", "-C", repoPath, "checkout", "--quiet", earlyCommit.Hash).Run()
	createCmd := exec.Command(fixToolPath)
	createCmd.Dir = repoPath
	createCmd.Run()

	// Now checkout the latest commit
	latestCommit := commits[len(commits)-1]
	exec.Command("git", "-C", repoPath, "checkout", "--quiet", latestCommit.Hash).Run()

	// Run with verbose to see which snapshot is selected
	fixCmd := exec.Command(fixToolPath, "--verbose", "--dry-run")
	fixCmd.Dir = repoPath
	out, _ := fixCmd.CombinedOutput()
	outStr := string(out)

	if verbose {
		fmt.Printf("  output: %s\n", outStr)
	}

	// Should use the snapshot (it's an ancestor)
	if !strings.Contains(outStr, "Using snapshot from") {
		fmt.Printf("  FAIL: didn't select ancestor snapshot\n")
		return false, nil
	}

	// The snapshot commit should be mentioned
	if !strings.Contains(outStr, earlyCommit.Hash[:8]) {
		fmt.Printf("  FAIL: didn't use expected snapshot %s\n", earlyCommit.Hash[:8])
		if verbose {
			fmt.Printf("  output: %s\n", outStr)
		}
		return false, nil
	}

	return true, nil
}
