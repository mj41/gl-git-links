package main

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mj41/gl-git-links/pkg/version"
)

// =============================================================================
// CLI Flags
// gl:docs/spec/git-glfix/incremental-tracking.md#L138 - CLI options specification
// =============================================================================

var (
	dryRun         bool
	verbose        bool
	jsonOutput     bool
	updateModified bool
	showVersion    bool
)

// =============================================================================
// Data Structures
// gl:docs/spec/git-glfix/incremental-tracking.md#L56 - Data structures
// =============================================================================

// SnapshotIndex is the registry of all snapshots
// gl:docs/spec/git-glfix/incremental-tracking.md#L60
type SnapshotIndex struct {
	Version   int             `json:"version"`
	Snapshots []SnapshotEntry `json:"snapshots"`
}

// SnapshotEntry is a summary of a snapshot in the index
type SnapshotEntry struct {
	CommitSHA string   `json:"commit_sha"`
	Branch    string   `json:"branch"`
	Timestamp string   `json:"timestamp"`
	Tags      []string `json:"tags,omitempty"`
}

// Snapshot represents the full state at a commit
// gl:docs/spec/git-glfix/incremental-tracking.md#L67
type Snapshot struct {
	CommitSHA string `json:"commit_sha"`
	Branch    string `json:"branch"`
	Timestamp string `json:"timestamp"`
	Links     []Link `json:"links"`
}

// Link represents a gl: link instance
// gl:docs/spec/git-glfix/incremental-tracking.md#L75
type Link struct {
	ID                 int     `json:"id"`
	SourceFile         string  `json:"source_file"`
	SourceLine         int     `json:"source_line"`
	TargetFile         string  `json:"target_file"`
	TargetFileOriginal string  `json:"target_file_original"`
	TrackedLine        int     `json:"tracked_line"`
	TrackedEndLine     int     `json:"tracked_end_line,omitempty"` // For ranges like #L10-L20
	LineContentHash    string  `json:"line_content_hash"`
	History            History `json:"history"`
}

// History tracks origin and modifications
// gl:docs/spec/git-glfix/incremental-tracking.md#L84
type History struct {
	OriginCommit  string         `json:"origin_commit"`
	OriginLine    int            `json:"origin_line"`
	OriginEndLine int            `json:"origin_end_line,omitempty"` // For ranges
	Modifications []Modification `json:"modifications,omitempty"`
	Renames       []Rename       `json:"renames,omitempty"`
}

// Modification records when target content changed significantly
type Modification struct {
	Commit        string `json:"commit"`
	ChangePercent int    `json:"change_percent"`
}

// Rename records when target file was renamed/moved
type Rename struct {
	Commit  string `json:"commit"`
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path"`
}

// UpdateResult represents the result of processing a link
// gl:docs/spec/git-glfix/overview.md#L45
type UpdateResult struct {
	Link       *Link
	OldLine    int
	NewLine    int
	OldEndLine int    // For ranges
	NewEndLine int    // For ranges
	Status     string // "updated", "unchanged", "broken", "error", "modified"
	Message    string
	ModPercent int // For "modified" status, the change percentage
}

// Config holds tool configuration from git config
// gl:docs/spec/git-glfix/incremental-tracking.md#L34
type Config struct {
	TrackBranches         []string
	TrackFeatureBranches  int
	RetentionDays         int
	RetentionMonths       int
	ModificationThreshold int
	RenameThreshold       int
}

// =============================================================================
// Constants
// =============================================================================

const (
	glLinksDir      = ".git/gl-links"
	snapshotsDir    = ".git/gl-links/snapshots"
	indexFile       = ".git/gl-links/index.json"
	indexVersion    = 1
	defaultRetDays  = 7
	defaultRetMonth = 1
	defaultModThres = 30
	defaultRenThres = 50
)

// =============================================================================
// Main Entry Point
// =============================================================================

func main() {
	flag.Usage = printUsage
	flag.BoolVar(&showVersion, "version", false, "Show version information")
	flag.BoolVar(&dryRun, "dry-run", false, "Print what would be changed without modifying files")
	flag.BoolVar(&verbose, "verbose", false, "Show detailed tracking information")
	flag.BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	flag.BoolVar(&updateModified, "update-modified", false, "Update links even if source file has uncommitted changes")
	flag.Parse()

	// Handle version flag
	if showVersion {
		version.ShowVersion()
		os.Exit(0)
	}

	// Check for subcommands
	args := flag.Args()
	if len(args) > 0 {
		switch args[0] {
		case "help":
			if len(args) > 1 {
				printSubcommandHelp(args[1])
			} else {
				printUsage()
			}
			return
		case "cache":
			runCacheCommand(args[1:])
			return
		case "config":
			runConfigCommand(args[1:])
			return
		case "status":
			runStatusCommand()
			return
		case "validate":
			runValidateCommand()
			return
		}
	}

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `usage: git-glfix [<options>] [<command> [<args>]]

Maintain gl: links in a git repository by tracking line number changes.

Options:
    --dry-run          Print what would be changed without modifying files
    --verbose          Show detailed tracking information
    --json             Output in JSON format
    --update-modified  Update links even if source file has uncommitted changes
    --version          Print version, commit and build information
    --help             Print this help

A link may address a single line (#L10) or a range (#L10-L20). Both ends of a
range are tracked, and --json reports them as old_end_line and new_end_line.

Commands:
    status      Show repository and snapshot status
    validate    Check for broken links without making changes
    cache       Manage the snapshot cache
    config      View or modify configuration

Run 'git-glfix help <command>' for more information on a specific command.
`)
}

func printSubcommandHelp(cmd string) {
	switch cmd {
	case "status":
		fmt.Fprintf(os.Stderr, `usage: git-glfix status

Show current repository state and snapshot information.

Output includes:
    - Current branch and HEAD commit
    - Active snapshot (if any) and how many commits behind HEAD
    - Number of links in worktree
`)
	case "validate":
		fmt.Fprintf(os.Stderr, `usage: git-glfix validate

Scan for broken links without making changes.

Reports links where:
    - Target file doesn't exist
    - Target line is out of range
`)
	case "cache":
		fmt.Fprintf(os.Stderr, `usage: git-glfix cache <subcommand>

Manage the snapshot cache stored in .git/gl-links/

Subcommands:
    list              List all cached snapshots
    show <commit>     Show details of a specific snapshot
    clear             Remove all cached snapshots
    prune             Remove old snapshots per retention policy
    rebuild           Rebuild snapshot for current HEAD
`)
	case "config":
		fmt.Fprintf(os.Stderr, `usage: git-glfix config <subcommand>

View or modify configuration.

Subcommands:
    list              Show current configuration
    set <key> <value> Set a configuration value
    unset <key>       Remove a configuration value

Configuration keys:
    retention-days           Days to keep daily snapshots (default: 7)
    retention-months         Months to keep monthly snapshots (default: 1)
    modification-threshold   Percentage change to trigger warning (default: 30)
    rename-threshold         Similarity threshold for rename detection (default: 50)
`)
	default:
		fmt.Fprintf(os.Stderr, "git-glfix: '%s' is not a git-glfix command. See 'git-glfix --help'.\n", cmd)
		os.Exit(1)
	}
}

// =============================================================================
// Main Workflow
// gl:docs/spec/git-glfix/incremental-tracking.md#L89 - Algorithm Workflow
// =============================================================================

func run() error {
	// Step 1: Verify git repo and get context
	repoRoot, err := getRepoRoot()
	if err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}

	if err := os.Chdir(repoRoot); err != nil {
		return fmt.Errorf("cannot change to repo root: %w", err)
	}

	head, err := getHeadCommit()
	if err != nil {
		return fmt.Errorf("cannot get HEAD: %w", err)
	}

	branch, err := getCurrentBranch()
	if err != nil {
		branch = "HEAD" // detached head
	}

	config := loadConfig()

	if verbose {
		fmt.Printf("Repository: %s\n", repoRoot)
		fmt.Printf("Branch: %s, HEAD: %s\n", branch, head[:8])
	}

	// Step 2: Snapshot Selection
	// gl:docs/spec/git-glfix/incremental-tracking.md#L91
	baseSnapshot, err := selectBestSnapshot(head, branch)
	if err != nil && verbose {
		fmt.Printf("No valid snapshot found, starting fresh: %v\n", err)
	}

	var currentLinks []Link
	var baseCommit string

	if baseSnapshot != nil {
		currentLinks = baseSnapshot.Links
		baseCommit = baseSnapshot.CommitSHA
		if verbose {
			fmt.Printf("Using snapshot from %s (%d links)\n", baseCommit[:8], len(currentLinks))
		}
	}

	// Step 3: Incremental Tracking from base to HEAD
	// gl:docs/spec/git-glfix/incremental-tracking.md#L100
	if baseCommit != "" && baseCommit != head {
		currentLinks, err = trackIncrementally(currentLinks, baseCommit, head, config)
		if err != nil {
			return fmt.Errorf("incremental tracking failed: %w", err)
		}
	}

	// Step 4: Discover new links and reconcile
	// gl:docs/spec/git-glfix/incremental-tracking.md#L153
	discoveredLinks, err := discoverLinks()
	if err != nil {
		return fmt.Errorf("link discovery failed: %w", err)
	}

	currentLinks, results := reconcileLinks(currentLinks, discoveredLinks, head, config)

	// Step 5: Track to worktree
	// gl:docs/spec/git-glfix/incremental-tracking.md#L167
	results = trackToWorktree(currentLinks, results)

	// Step 6: Apply updates
	if err := applyUpdates(results); err != nil {
		return fmt.Errorf("failed to apply updates: %w", err)
	}

	// Step 7: Save snapshot (unless dry-run)
	if !dryRun {
		if err := saveSnapshot(head, branch, currentLinks); err != nil {
			if verbose {
				fmt.Printf("Warning: failed to save snapshot: %v\n", err)
			}
		}
		pruneSnapshots(config)
	}

	// Step 8: Report results
	reportResults(results)

	return nil
}

// =============================================================================
// Git Helpers
// =============================================================================

func getRepoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func getHeadCommit() (string, error) {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func getCurrentBranch() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "", err
	}
	branch := strings.TrimSpace(string(out))
	if branch == "HEAD" {
		return "", fmt.Errorf("detached HEAD")
	}
	return branch, nil
}

func isAncestor(ancestor, descendant string) bool {
	err := exec.Command("git", "merge-base", "--is-ancestor", ancestor, descendant).Run()
	return err == nil
}

func getCommitsBetween(from, to string) ([]string, error) {
	if from == "" {
		// No base, get all commits up to 'to'
		out, err := exec.Command("git", "log", "--format=%H", "--reverse", to).Output()
		if err != nil {
			return nil, err
		}
		return parseLines(out), nil
	}

	out, err := exec.Command("git", "log", "--format=%H", "--reverse", from+".."+to).Output()
	if err != nil {
		return nil, err
	}
	return parseLines(out), nil
}

func parseLines(data []byte) []string {
	var lines []string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func getTrackedFiles() ([]string, error) {
	out, err := exec.Command("git", "ls-files").Output()
	if err != nil {
		return nil, err
	}
	return parseLines(out), nil
}

func getFileAtCommit(commit, filePath string) (string, error) {
	out, err := exec.Command("git", "show", commit+":"+filePath).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func getFileDiff(fromCommit, toCommit, filePath string) (string, error) {
	out, err := exec.Command("git", "diff", "-U0", fromCommit, toCommit, "--", filePath).Output()
	if err != nil {
		// Diff returns error on binary files or if file doesn't exist
		return "", err
	}
	return string(out), nil
}

// =============================================================================
// Configuration
// gl:docs/spec/git-glfix/incremental-tracking.md#L34
// =============================================================================

func loadConfig() Config {
	config := Config{
		TrackBranches:         []string{"main"},
		TrackFeatureBranches:  5,
		RetentionDays:         defaultRetDays,
		RetentionMonths:       defaultRetMonth,
		ModificationThreshold: defaultModThres,
		RenameThreshold:       defaultRenThres,
	}

	// Read from git config
	if out, err := exec.Command("git", "config", "--get-all", "gl-links.track-branch").Output(); err == nil {
		config.TrackBranches = parseLines(out)
	}

	if out, err := exec.Command("git", "config", "--get", "gl-links.track-feature-branches").Output(); err == nil {
		if v, err := strconv.Atoi(strings.TrimSpace(string(out))); err == nil {
			config.TrackFeatureBranches = v
		}
	}

	if out, err := exec.Command("git", "config", "--get", "gl-links.retention-days").Output(); err == nil {
		if v, err := strconv.Atoi(strings.TrimSpace(string(out))); err == nil {
			config.RetentionDays = v
		}
	}

	if out, err := exec.Command("git", "config", "--get", "gl-links.modification-threshold").Output(); err == nil {
		if v, err := strconv.Atoi(strings.TrimSpace(string(out))); err == nil {
			config.ModificationThreshold = v
		}
	}

	if out, err := exec.Command("git", "config", "--get", "gl-links.rename-threshold").Output(); err == nil {
		if v, err := strconv.Atoi(strings.TrimSpace(string(out))); err == nil {
			config.RenameThreshold = v
		}
	}

	return config
}

// =============================================================================
// Snapshot Storage
// gl:docs/spec/git-glfix/incremental-tracking.md#L17
// =============================================================================

func ensureStorageDir() error {
	return os.MkdirAll(snapshotsDir, 0755)
}

func loadIndex() (*SnapshotIndex, error) {
	data, err := os.ReadFile(indexFile)
	if err != nil {
		if os.IsNotExist(err) {
			return &SnapshotIndex{Version: indexVersion}, nil
		}
		return nil, err
	}

	var index SnapshotIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, err
	}
	return &index, nil
}

func saveIndex(index *SnapshotIndex) error {
	if err := ensureStorageDir(); err != nil {
		return err
	}

	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(indexFile, data, 0644)
}

func loadSnapshot(commitSHA string) (*Snapshot, error) {
	path := filepath.Join(snapshotsDir, commitSHA+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

func saveSnapshot(commitSHA, branch string, links []Link) error {
	if err := ensureStorageDir(); err != nil {
		return err
	}

	snapshot := Snapshot{
		CommitSHA: commitSHA,
		Branch:    branch,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Links:     links,
	}

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(snapshotsDir, commitSHA+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}

	// Update index
	index, err := loadIndex()
	if err != nil {
		index = &SnapshotIndex{Version: indexVersion}
	}

	// Remove existing entry for this commit
	var newSnapshots []SnapshotEntry
	for _, s := range index.Snapshots {
		if s.CommitSHA != commitSHA {
			newSnapshots = append(newSnapshots, s)
		}
	}

	newSnapshots = append(newSnapshots, SnapshotEntry{
		CommitSHA: commitSHA,
		Branch:    branch,
		Timestamp: snapshot.Timestamp,
		Tags:      []string{"head"},
	})

	index.Snapshots = newSnapshots
	return saveIndex(index)
}

// selectBestSnapshot finds the youngest ancestor snapshot of HEAD
// gl:docs/spec/git-glfix/incremental-tracking.md#L91
func selectBestSnapshot(head, branch string) (*Snapshot, error) {
	index, err := loadIndex()
	if err != nil {
		return nil, err
	}

	if len(index.Snapshots) == 0 {
		return nil, fmt.Errorf("no snapshots found")
	}

	// Find candidates - snapshots that are ancestors of HEAD
	type candidate struct {
		entry    SnapshotEntry
		distance int // commits between snapshot and HEAD
	}

	var candidates []candidate
	for _, entry := range index.Snapshots {
		// Check if snapshot commit is ancestor of HEAD
		if isAncestor(entry.CommitSHA, head) {
			// Count commits between
			commits, err := getCommitsBetween(entry.CommitSHA, head)
			if err != nil {
				continue
			}
			candidates = append(candidates, candidate{entry, len(commits)})
		}
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no ancestor snapshot found")
	}

	// Find the one with minimum distance (youngest ancestor)
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.distance < best.distance {
			best = c
		}
	}

	return loadSnapshot(best.entry.CommitSHA)
}

// =============================================================================
// Incremental Tracking
// gl:docs/spec/git-glfix/incremental-tracking.md#L100
// =============================================================================

func trackIncrementally(links []Link, fromCommit, toCommit string, config Config) ([]Link, error) {
	commits, err := getCommitsBetween(fromCommit, toCommit)
	if err != nil {
		return links, fmt.Errorf("cannot get commits: %w", err)
	}

	if verbose {
		fmt.Printf("Processing %d commits from %s to %s\n", len(commits), fromCommit[:8], toCommit[:8])
	}

	// Build map of target files to links
	targetToLinks := make(map[string][]*Link)
	for i := range links {
		targetToLinks[links[i].TargetFile] = append(targetToLinks[links[i].TargetFile], &links[i])
	}

	prevCommit := fromCommit
	for _, commit := range commits {
		if err := processCommitDiff(targetToLinks, prevCommit, commit, config); err != nil {
			if verbose {
				fmt.Printf("  Warning: commit %s: %v\n", commit[:8], err)
			}
		}
		prevCommit = commit
	}

	return links, nil
}

func processCommitDiff(targetToLinks map[string][]*Link, fromCommit, toCommit string, config Config) error {
	// Get changed files in this commit
	out, err := exec.Command("git", "diff", "--name-only", fromCommit, toCommit).Output()
	if err != nil {
		return err
	}

	changedFiles := parseLines(out)

	for _, filePath := range changedFiles {
		linksForFile := targetToLinks[filePath]
		if len(linksForFile) == 0 {
			continue // No links point to this file
		}

		if err := updateLinksForFileDiff(linksForFile, fromCommit, toCommit, filePath, config); err != nil {
			if verbose {
				fmt.Printf("    Warning: %s: %v\n", filePath, err)
			}
		}
	}

	return nil
}

func updateLinksForFileDiff(links []*Link, fromCommit, toCommit, filePath string, config Config) error {
	diffStr, err := getFileDiff(fromCommit, toCommit, filePath)
	if err != nil {
		return err
	}

	// Parse diff hunks
	hunks := parseDiffHunks(diffStr)

	// Get old and new file content for hash comparison
	oldContent, _ := getFileAtCommit(fromCommit, filePath)
	newContent, _ := getFileAtCommit(toCommit, filePath)
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	for _, link := range links {
		oldLine := link.TrackedLine
		newLine := applyHunksToLine(oldLine, hunks)

		if newLine <= 0 {
			// Line was deleted
			if verbose {
				fmt.Printf("    Line %d deleted in %s\n", oldLine, toCommit[:8])
			}
			// Mark as broken - could attempt heuristic recovery here
			continue
		}

		// Track end line if this is a range
		if link.TrackedEndLine > 0 {
			oldEndLine := link.TrackedEndLine
			newEndLine := applyHunksToLine(oldEndLine, hunks)

			if newEndLine <= 0 {
				// End line was deleted
				if verbose {
					fmt.Printf("    End line %d deleted in %s\n", oldEndLine, toCommit[:8])
				}
				continue
			}

			link.TrackedEndLine = newEndLine
		}

		// Check for content modification
		if newLine != oldLine || contentChanged(oldLines, newLines, oldLine, newLine, link.LineContentHash, config) {
			if newLine > 0 && newLine <= len(newLines) {
				newHash := hashLine(newLines[newLine-1])
				if link.LineContentHash != "" && link.LineContentHash != newHash {
					// Content changed
					changePct := calculateChangePercent(oldLines, newLines, oldLine, newLine)
					if changePct >= config.ModificationThreshold {
						link.History.Modifications = append(link.History.Modifications, Modification{
							Commit:        toCommit,
							ChangePercent: changePct,
						})
					}
				}
				link.LineContentHash = newHash
			}
		}

		link.TrackedLine = newLine
	}

	return nil
}

// parseDiffHunks extracts hunk information from unified diff
func parseDiffHunks(diff string) []Hunk {
	var hunks []Hunk
	lines := strings.Split(diff, "\n")

	for _, line := range lines {
		if strings.HasPrefix(line, "@@") {
			hunk := parseHunkHeader(line)
			if hunk != nil {
				hunks = append(hunks, *hunk)
			}
		}
	}

	return hunks
}

type Hunk struct {
	OldStart int
	OldCount int
	NewStart int
	NewCount int
}

func parseHunkHeader(line string) *Hunk {
	// Format: @@ -oldStart,oldCount +newStart,newCount @@
	re := regexp.MustCompile(`@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)
	matches := re.FindStringSubmatch(line)
	if len(matches) < 4 {
		return nil
	}

	oldStart, _ := strconv.Atoi(matches[1])
	oldCount := 1
	if matches[2] != "" {
		oldCount, _ = strconv.Atoi(matches[2])
	}

	newStart, _ := strconv.Atoi(matches[3])
	newCount := 1
	if matches[4] != "" {
		newCount, _ = strconv.Atoi(matches[4])
	}

	return &Hunk{
		OldStart: oldStart,
		OldCount: oldCount,
		NewStart: newStart,
		NewCount: newCount,
	}
}

// applyHunksToLine calculates new line number after applying diff hunks
func applyHunksToLine(oldLine int, hunks []Hunk) int {
	offset := 0

	for _, h := range hunks {
		if oldLine < h.OldStart {
			// Line is before this hunk, no effect
			continue
		}

		if oldLine < h.OldStart+h.OldCount {
			// Line is within the deleted range
			if h.OldCount == 0 {
				// Pure insertion before this line
				offset += h.NewCount
			} else {
				// Line was deleted
				return -1
			}
			continue
		}

		// Line is after this hunk, apply offset
		offset += h.NewCount - h.OldCount
	}

	return oldLine + offset
}

// =============================================================================
// Link Discovery
// gl:docs/spec/git-glfix/overview.md#L26
// =============================================================================

func discoverLinks() ([]Link, error) {
	files, err := getTrackedFiles()
	if err != nil {
		return nil, err
	}

	// Match gl:path#L10 or gl:path#L10-L20 (ranges)
	re := regexp.MustCompile(`gl:(\S+?)#L(\d+)(?:-L(\d+))?`)
	var links []Link
	nextID := 1

	for _, file := range files {
		if isBinaryFile(file) {
			continue
		}

		content, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		lines := strings.Split(string(content), "\n")
		for lineNum, line := range lines {
			matches := re.FindAllStringSubmatch(line, -1)
			for _, match := range matches {
				if len(match) < 3 {
					continue
				}

				targetPathRaw := match[1]
				targetLine, _ := strconv.Atoi(match[2])

				// Check for end line in range (e.g., #L10-L20)
				var targetEndLine int
				if len(match) > 3 && match[3] != "" {
					targetEndLine, _ = strconv.Atoi(match[3])
				}

				targetPath := resolvePath(file, targetPathRaw)

				links = append(links, Link{
					ID:                 nextID,
					SourceFile:         file,
					SourceLine:         lineNum + 1,
					TargetFile:         targetPath,
					TargetFileOriginal: targetPathRaw,
					TrackedLine:        targetLine,
					TrackedEndLine:     targetEndLine,
				})
				nextID++
			}
		}
	}

	return links, nil
}

func isBinaryFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return true
	}
	defer f.Close()

	buf := make([]byte, 8192)
	n, _ := f.Read(buf)
	for i := 0; i < n; i++ {
		if buf[i] == 0 {
			return true
		}
	}
	return false
}

func resolvePath(sourceFile, targetPathRaw string) string {
	if strings.HasPrefix(targetPathRaw, "./") || strings.HasPrefix(targetPathRaw, "../") {
		dir := filepath.Dir(sourceFile)
		resolved := filepath.Clean(filepath.Join(dir, targetPathRaw))
		return resolved
	}
	return targetPathRaw
}

// =============================================================================
// Link Reconciliation
// =============================================================================

func reconcileLinks(tracked, discovered []Link, head string, config Config) ([]Link, []UpdateResult) {
	var results []UpdateResult

	// Build lookup map for tracked links
	// Key on SourceFile+SourceLine+TargetFile only - end lines change during tracking
	type linkKey struct {
		SourceFile string
		SourceLine int
		TargetFile string
	}

	trackedMap := make(map[linkKey]*Link)
	for i := range tracked {
		key := linkKey{
			SourceFile: tracked[i].SourceFile,
			SourceLine: tracked[i].SourceLine,
			TargetFile: tracked[i].TargetFile,
		}
		trackedMap[key] = &tracked[i]
	}

	var reconciled []Link
	nextID := len(tracked) + 1

	for _, disc := range discovered {
		key := linkKey{
			SourceFile: disc.SourceFile,
			SourceLine: disc.SourceLine,
			TargetFile: disc.TargetFile,
		}

		if existing, ok := trackedMap[key]; ok {
			// Link exists in tracking
			result := UpdateResult{
				Link:       existing,
				OldLine:    disc.TrackedLine, // What's written in source
				NewLine:    existing.TrackedLine,
				OldEndLine: disc.TrackedEndLine,
				NewEndLine: existing.TrackedEndLine,
			}

			// Check if tracked link is broken
			if existing.TrackedLine == -1 {
				result.Status = "broken"
				result.NewLine = disc.TrackedLine // Keep original for display
				result.NewEndLine = disc.TrackedEndLine
				if _, err := os.Stat(disc.TargetFile); os.IsNotExist(err) {
					result.Message = fmt.Sprintf("target file does not exist: %s", disc.TargetFile)
				} else {
					result.Message = "target line deleted or out of range"
				}
			} else {
				startChanged := disc.TrackedLine != existing.TrackedLine
				endChanged := disc.TrackedEndLine != 0 && disc.TrackedEndLine != existing.TrackedEndLine

				if startChanged || endChanged {
					result.Status = "updated"
					if disc.TrackedEndLine > 0 {
						result.Message = fmt.Sprintf("L%d-L%d -> L%d-L%d", disc.TrackedLine, disc.TrackedEndLine, existing.TrackedLine, existing.TrackedEndLine)
					} else {
						result.Message = fmt.Sprintf("L%d -> L%d", disc.TrackedLine, existing.TrackedLine)
					}
				} else {
					result.Status = "unchanged"
				}
			}

			// Check for modifications
			if len(existing.History.Modifications) > 0 {
				lastMod := existing.History.Modifications[len(existing.History.Modifications)-1]
				if lastMod.ChangePercent >= config.ModificationThreshold {
					result.Status = "modified"
					result.ModPercent = lastMod.ChangePercent
					result.Message = fmt.Sprintf("content changed %d%% in %s", lastMod.ChangePercent, lastMod.Commit[:8])
				}
			}

			results = append(results, result)
			reconciled = append(reconciled, *existing)
			delete(trackedMap, key)
		} else {
			// New link - need to track from origin
			newLink := initializeNewLink(disc, head, nextID)
			nextID++

			result := UpdateResult{
				Link:       &newLink,
				OldLine:    disc.TrackedLine,
				NewLine:    newLink.TrackedLine,
				OldEndLine: disc.TrackedEndLine,
				NewEndLine: newLink.TrackedEndLine,
				Status:     "unchanged",
			}

			// Check if link is broken (TrackedLine == -1)
			if newLink.TrackedLine == -1 {
				result.Status = "broken"
				result.NewLine = disc.TrackedLine // Keep original for display
				result.NewEndLine = disc.TrackedEndLine
				if _, err := os.Stat(disc.TargetFile); os.IsNotExist(err) {
					result.Message = fmt.Sprintf("target file does not exist: %s", disc.TargetFile)
				} else {
					result.Message = "target line deleted or out of range"
				}
			} else {
				startChanged := disc.TrackedLine != newLink.TrackedLine
				endChanged := disc.TrackedEndLine != 0 && disc.TrackedEndLine != newLink.TrackedEndLine

				if startChanged || endChanged {
					result.Status = "updated"
					if disc.TrackedEndLine > 0 {
						result.Message = fmt.Sprintf("L%d-L%d -> L%d-L%d", disc.TrackedLine, disc.TrackedEndLine, newLink.TrackedLine, newLink.TrackedEndLine)
					} else {
						result.Message = fmt.Sprintf("L%d -> L%d", disc.TrackedLine, newLink.TrackedLine)
					}
				}
			}

			results = append(results, result)
			reconciled = append(reconciled, newLink)
		}
	}

	// Links in trackedMap that weren't found are deleted
	// (we don't need to do anything with them)

	return reconciled, results
}

func initializeNewLink(disc Link, head string, id int) Link {
	link := disc
	link.ID = id

	// Check if target file exists
	if _, err := os.Stat(disc.TargetFile); os.IsNotExist(err) {
		// Target file doesn't exist - mark as broken by setting TrackedLine to -1
		link.TrackedLine = -1
		link.History = History{
			OriginCommit:  head,
			OriginLine:    disc.TrackedLine,
			OriginEndLine: disc.TrackedEndLine,
		}
		return link
	}

	// Get origin commit
	origin, err := getOriginCommit(disc.SourceFile, disc.SourceLine)
	if err != nil || strings.HasPrefix(origin, "0000000") {
		origin = head
	}

	link.History = History{
		OriginCommit:  origin,
		OriginLine:    disc.TrackedLine,
		OriginEndLine: disc.TrackedEndLine,
	}

	// Track from origin to HEAD if different
	if origin != head {
		newLine, err := trackLineToHead(disc.TargetFile, disc.TrackedLine, origin, head)
		if err != nil {
			// Tracking failed - line was deleted or file doesn't exist at origin
			link.TrackedLine = -1
			return link
		}
		link.TrackedLine = newLine

		// Track end line if this is a range
		if disc.TrackedEndLine > 0 {
			newEndLine, err := trackLineToHead(disc.TargetFile, disc.TrackedEndLine, origin, head)
			if err != nil {
				// End line tracking failed - mark as broken
				link.TrackedLine = -1
				return link
			}
			link.TrackedEndLine = newEndLine
		}
	}

	// Verify tracked line exists in current file
	content, err := os.ReadFile(disc.TargetFile)
	if err != nil {
		link.TrackedLine = -1
		return link
	}

	lines := strings.Split(string(content), "\n")
	if link.TrackedLine <= 0 || link.TrackedLine > len(lines) {
		// Line out of range - broken
		link.TrackedLine = -1
		return link
	}

	// Verify end line if this is a range
	if link.TrackedEndLine > 0 {
		if link.TrackedEndLine <= 0 || link.TrackedEndLine > len(lines) {
			link.TrackedLine = -1
			return link
		}
		// Validate range is sensible (start <= end)
		if link.TrackedLine > link.TrackedEndLine {
			link.TrackedLine = -1
			return link
		}
	}

	// Compute content hash
	link.LineContentHash = hashLine(lines[link.TrackedLine-1])

	return link
}

func getOriginCommit(filePath string, lineNum int) (string, error) {
	cmd := exec.Command("git", "blame", "-L", fmt.Sprintf("%d,%d", lineNum, lineNum), "--porcelain", "--", filePath)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	if scanner.Scan() {
		parts := strings.Fields(scanner.Text())
		if len(parts) > 0 {
			return parts[0], nil
		}
	}
	return "", fmt.Errorf("no blame output")
}

func trackLineToHead(targetFile string, targetLine int, originCommit, headCommit string) (int, error) {
	if originCommit == headCommit {
		return targetLine, nil
	}

	rangeArg := fmt.Sprintf("%s..%s", originCommit, headCommit)
	cmd := exec.Command("git", "blame", "--reverse", rangeArg, "-L", fmt.Sprintf("%d,%d", targetLine, targetLine), "--porcelain", "--", targetFile)
	out, err := cmd.Output()
	if err != nil {
		return -1, fmt.Errorf("reverse blame failed: %w", err)
	}

	// If output is empty, the line was deleted
	if len(bytes.TrimSpace(out)) == 0 {
		return -1, fmt.Errorf("line deleted")
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	if scanner.Scan() {
		parts := strings.Fields(scanner.Text())
		if len(parts) >= 2 {
			finalCommit := parts[0]
			lineInFinal, _ := strconv.Atoi(parts[1])

			// If the final commit is not HEAD, line was modified/deleted at some point
			// Try heuristic recovery
			if finalCommit != headCommit {
				recoveredLine, err := recoverLostLine(targetFile, lineInFinal, finalCommit, headCommit)
				if err != nil {
					return -1, fmt.Errorf("line deleted at %s: %w", finalCommit[:8], err)
				}
				return recoveredLine, nil
			}

			return lineInFinal, nil
		}
	}

	return -1, fmt.Errorf("no result from reverse blame")
}

// recoverLostLine attempts heuristic recovery when git loses track of a line
func recoverLostLine(filePath string, lineInStoppedCommit int, stoppedCommit string, headCommit string) (int, error) {
	// Find next commit after stoppedCommit
	cmd := exec.Command("git", "log", "--format=%H", "-n", "1", "--reverse", fmt.Sprintf("%s..%s", stoppedCommit, headCommit), "--", filePath)
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return -1, fmt.Errorf("could not find next commit")
	}
	nextCommit := strings.TrimSpace(string(out))

	// Get diff between stopped and next commit
	cmdDiff := exec.Command("git", "diff", "-U0", stoppedCommit, nextCommit, "--", filePath)
	outDiff, err := cmdDiff.Output()
	if err != nil {
		return -1, err
	}

	// Parse diff to find what happened to the line
	deleted, added, addedNums := parseDiffForLine(string(outDiff), lineInStoppedCommit)

	if deleted == "" {
		return -1, fmt.Errorf("line %d not found in diff", lineInStoppedCommit)
	}

	// Find best match using similarity
	bestScore := 0.0
	bestLine := -1
	bestIdx := -1

	for i, addedLine := range added {
		score := similarity(deleted, addedLine)
		if score > bestScore {
			bestScore = score
			bestLine = addedNums[i]
			bestIdx = i
		}
	}

	if bestScore > 0.6 {
		if verbose {
			fmt.Printf("    Heuristic recovery: '%s' -> '%s' (%.0f%%)\n",
				strings.TrimSpace(deleted),
				strings.TrimSpace(added[bestIdx]),
				bestScore*100)
		}
		// Continue tracking from nextCommit
		if nextCommit == headCommit {
			return bestLine, nil
		}
		return trackLineToHead(filePath, bestLine, nextCommit, headCommit)
	}

	return -1, fmt.Errorf("no similar line found (best %.0f%%)", bestScore*100)
}

// parseDiffForLine extracts deleted and added lines from a diff hunk
func parseDiffForLine(diff string, targetLine int) (deleted string, added []string, addedNums []int) {
	lines := strings.Split(diff, "\n")

	var oldLineStart, oldLineCount, newLineStart int
	inHunk := false
	currentOldLine, currentNewLine := 0, 0

	for _, line := range lines {
		if strings.HasPrefix(line, "@@") {
			inHunk = false
			re := regexp.MustCompile(`@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)
			matches := re.FindStringSubmatch(line)
			if len(matches) < 4 {
				continue
			}

			oldLineStart, _ = strconv.Atoi(matches[1])
			oldLineCount = 1
			if matches[2] != "" {
				oldLineCount, _ = strconv.Atoi(matches[2])
			}

			newLineStart, _ = strconv.Atoi(matches[3])

			if targetLine >= oldLineStart && targetLine < oldLineStart+oldLineCount {
				inHunk = true
				currentOldLine = oldLineStart
				currentNewLine = newLineStart
				added = nil
				addedNums = nil
				deleted = ""
			}
			continue
		}

		if inHunk {
			if strings.HasPrefix(line, "-") {
				if currentOldLine == targetLine {
					deleted = strings.TrimPrefix(line, "-")
				}
				currentOldLine++
			} else if strings.HasPrefix(line, "+") {
				added = append(added, strings.TrimPrefix(line, "+"))
				addedNums = append(addedNums, currentNewLine)
				currentNewLine++
			}
		}
	}

	return deleted, added, addedNums
}

// =============================================================================
// Worktree Tracking
// gl:docs/spec/git-glfix/incremental-tracking.md#L167
// =============================================================================

func trackToWorktree(links []Link, results []UpdateResult) []UpdateResult {
	// For each link, check if target file has uncommitted changes
	// and adjust line number accordingly
	for i := range results {
		if results[i].Status == "broken" || results[i].Status == "error" {
			continue
		}

		link := results[i].Link

		// Track start line
		wtLine, err := trackHeadToWorkTree(link.TargetFile, link.TrackedLine)
		startChanged := err == nil && wtLine != link.TrackedLine

		// Track end line if this is a range
		var wtEndLine int
		endChanged := false
		if link.TrackedEndLine > 0 {
			wtEndLine, err = trackHeadToWorkTree(link.TargetFile, link.TrackedEndLine)
			endChanged = err == nil && wtEndLine != link.TrackedEndLine
		}

		if startChanged || endChanged {
			if startChanged {
				results[i].NewLine = wtLine
			}
			if endChanged {
				results[i].NewEndLine = wtEndLine
			}

			if results[i].Status == "unchanged" {
				needsUpdate := (startChanged && wtLine != results[i].OldLine) ||
					(endChanged && wtEndLine != results[i].OldEndLine)

				if needsUpdate {
					results[i].Status = "updated"
					if link.TrackedEndLine > 0 {
						results[i].Message = fmt.Sprintf("L%d-L%d -> L%d-L%d (worktree)",
							results[i].OldLine, results[i].OldEndLine, wtLine, wtEndLine)
					} else {
						results[i].Message = fmt.Sprintf("L%d -> L%d (worktree)", results[i].OldLine, wtLine)
					}
				}
			}
		}
	}

	return results
}

func trackHeadToWorkTree(filePath string, lineInHead int) (int, error) {
	// Check if file has uncommitted changes
	out, err := exec.Command("git", "diff", "--name-only", "--", filePath).Output()
	if err != nil || len(strings.TrimSpace(string(out))) == 0 {
		return lineInHead, nil // No changes
	}

	// Get diff from HEAD to worktree
	diffOut, err := exec.Command("git", "diff", "-U0", "HEAD", "--", filePath).Output()
	if err != nil {
		return lineInHead, err
	}

	hunks := parseDiffHunks(string(diffOut))
	newLine := applyHunksToLine(lineInHead, hunks)
	if newLine <= 0 {
		return lineInHead, fmt.Errorf("line deleted in worktree")
	}

	return newLine, nil
}

// =============================================================================
// Apply Updates
// =============================================================================

func applyUpdates(results []UpdateResult) error {
	fileUpdates := make(map[string][]UpdateResult)
	for _, r := range results {
		if r.Status == "updated" {
			fileUpdates[r.Link.SourceFile] = append(fileUpdates[r.Link.SourceFile], r)
		}
	}

	modifiedFiles := getModifiedFiles()

	for filePath, updates := range fileUpdates {
		if dryRun {
			if !jsonOutput {
				for _, u := range updates {
					if u.OldEndLine > 0 {
						fmt.Printf("[dry-run] %s:%d: gl:%s#L%d-L%d -> gl:%s#L%d-L%d\n",
							filePath, u.Link.SourceLine,
							u.Link.TargetFileOriginal, u.OldLine, u.OldEndLine,
							u.Link.TargetFileOriginal, u.NewLine, u.NewEndLine)
					} else {
						fmt.Printf("[dry-run] %s:%d: gl:%s#L%d -> gl:%s#L%d\n",
							filePath, u.Link.SourceLine,
							u.Link.TargetFileOriginal, u.OldLine,
							u.Link.TargetFileOriginal, u.NewLine)
					}
				}
			}
			continue
		}

		if modifiedFiles[filePath] && !updateModified {
			if !jsonOutput {
				fmt.Fprintf(os.Stderr, "Skipping %s: file has uncommitted changes (use --update-modified to override)\n", filePath)
			}
			continue
		}

		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", filePath, err)
		}

		lines := strings.Split(string(content), "\n")

		for _, u := range updates {
			lineIdx := u.Link.SourceLine - 1
			if lineIdx < 0 || lineIdx >= len(lines) {
				continue
			}

			var oldPattern, newPattern string
			if u.OldEndLine > 0 {
				// Range link: #L10-L20
				oldPattern = fmt.Sprintf("gl:%s#L%d-L%d", u.Link.TargetFileOriginal, u.OldLine, u.OldEndLine)
				newPattern = fmt.Sprintf("gl:%s#L%d-L%d", u.Link.TargetFileOriginal, u.NewLine, u.NewEndLine)
			} else {
				// Single line link: #L10
				oldPattern = fmt.Sprintf("gl:%s#L%d", u.Link.TargetFileOriginal, u.OldLine)
				newPattern = fmt.Sprintf("gl:%s#L%d", u.Link.TargetFileOriginal, u.NewLine)
			}
			lines[lineIdx] = strings.Replace(lines[lineIdx], oldPattern, newPattern, 1)
		}

		output := strings.Join(lines, "\n")
		if err := os.WriteFile(filePath, []byte(output), 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", filePath, err)
		}

		if !jsonOutput {
			fmt.Printf("Updated %s (%d links)\n", filePath, len(updates))
		}
	}

	return nil
}

func getModifiedFiles() map[string]bool {
	modified := make(map[string]bool)

	out, err := exec.Command("git", "status", "--porcelain").Output()
	if err != nil {
		return modified
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 4 {
			continue
		}
		status := line[:2]
		filePath := strings.TrimSpace(line[3:])

		if strings.Contains(filePath, " -> ") {
			parts := strings.Split(filePath, " -> ")
			if len(parts) == 2 {
				filePath = parts[1]
			}
		}

		if status != "??" {
			modified[filePath] = true
		}
	}

	return modified
}

// =============================================================================
// Reporting
// =============================================================================

func reportResults(results []UpdateResult) {
	if jsonOutput {
		reportJSON(results)
		return
	}

	var updated, unchanged, broken, errors, modified int
	for _, r := range results {
		switch r.Status {
		case "updated":
			updated++
		case "unchanged":
			unchanged++
		case "broken":
			broken++
			fmt.Fprintf(os.Stderr, "Warning: Broken link in %s:%d - %s\n",
				r.Link.SourceFile, r.Link.SourceLine, r.Message)
		case "error":
			errors++
			if verbose {
				fmt.Fprintf(os.Stderr, "Error: %s:%d - %s\n",
					r.Link.SourceFile, r.Link.SourceLine, r.Message)
			}
		case "modified":
			modified++
			fmt.Fprintf(os.Stderr, "Warning: Modified content in %s:%d -> %s#L%d (%s)\n",
				r.Link.SourceFile, r.Link.SourceLine, r.Link.TargetFile, r.NewLine, r.Message)
		}
	}

	if verbose || updated > 0 || broken > 0 || modified > 0 {
		fmt.Printf("\nSummary: %d updated, %d unchanged, %d broken, %d modified, %d errors\n",
			updated, unchanged, broken, modified, errors)
	}
}

func reportJSON(results []UpdateResult) {
	type JSONResult struct {
		File    string `json:"file"`
		Line    int    `json:"line"`
		Target  string `json:"target"`
		OldLine int    `json:"old_line"`
		NewLine int    `json:"new_line"`
		// Ranges (#L10-L20) track both ends, but only the start reached
		// the JSON — the end survived solely inside the human-readable message,
		// as "L2-L4 -> L4-L6". Anything consuming --json, the test harness
		// included, could see a range move but not where it moved to. Omitted
		// for single-line links, which have no end.
		OldEndLine int    `json:"old_end_line,omitempty"`
		NewEndLine int    `json:"new_end_line,omitempty"`
		Status     string `json:"status"`
		Message    string `json:"message,omitempty"`
		ModPercent int    `json:"mod_percent,omitempty"`
	}

	var out []JSONResult
	for _, r := range results {
		out = append(out, JSONResult{
			File:       r.Link.SourceFile,
			Line:       r.Link.SourceLine,
			Target:     r.Link.TargetFile,
			OldLine:    r.OldLine,
			NewLine:    r.NewLine,
			OldEndLine: r.OldEndLine,
			NewEndLine: r.NewEndLine,
			Status:     r.Status,
			Message:    r.Message,
			ModPercent: r.ModPercent,
		})
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}

// =============================================================================
// Helper Functions
// =============================================================================

func hashLine(line string) string {
	h := md5.Sum([]byte(strings.TrimSpace(line)))
	return hex.EncodeToString(h[:])
}

func contentChanged(oldLines, newLines []string, oldLineNum, newLineNum int, oldHash string, config Config) bool {
	if oldLineNum <= 0 || oldLineNum > len(oldLines) {
		return true
	}
	if newLineNum <= 0 || newLineNum > len(newLines) {
		return true
	}

	newHash := hashLine(newLines[newLineNum-1])
	return oldHash != newHash
}

func calculateChangePercent(oldLines, newLines []string, oldLineNum, newLineNum int) int {
	if oldLineNum <= 0 || oldLineNum > len(oldLines) {
		return 100
	}
	if newLineNum <= 0 || newLineNum > len(newLines) {
		return 100
	}

	old := strings.TrimSpace(oldLines[oldLineNum-1])
	new := strings.TrimSpace(newLines[newLineNum-1])

	if old == new {
		return 0
	}

	sim := similarity(old, new)
	return int((1.0 - sim) * 100)
}

func similarity(s1, s2 string) float64 {
	s1 = strings.TrimSpace(s1)
	s2 = strings.TrimSpace(s2)
	if s1 == s2 {
		return 1.0
	}
	if len(s1) == 0 || len(s2) == 0 {
		return 0.0
	}

	d := levenshtein(s1, s2)
	maxLen := float64(max(len(s1), len(s2)))
	return 1.0 - (float64(d) / maxLen)
}

func levenshtein(s1, s2 string) int {
	r1, r2 := []rune(s1), []rune(s2)
	n, m := len(r1), len(r2)
	if n > m {
		r1, r2 = r2, r1
		n, m = m, n
	}

	curr := make([]int, n+1)
	for i := range curr {
		curr[i] = i
	}

	for i := 1; i <= m; i++ {
		prev := curr
		curr = make([]int, n+1)
		curr[0] = i
		for j := 1; j <= n; j++ {
			cost := 0
			if r1[j-1] != r2[i-1] {
				cost = 1
			}
			curr[j] = min(prev[j]+1, min(curr[j-1]+1, prev[j-1]+cost))
		}
	}
	return curr[n]
}

// =============================================================================
// Snapshot Pruning
// gl:docs/spec/git-glfix/incremental-tracking.md#L116
// =============================================================================

func pruneSnapshots(config Config) {
	index, err := loadIndex()
	if err != nil || len(index.Snapshots) == 0 {
		return
	}

	// For now, simple pruning: keep last N days worth
	cutoff := time.Now().AddDate(0, 0, -config.RetentionDays)
	var kept []SnapshotEntry

	for _, s := range index.Snapshots {
		ts, err := time.Parse(time.RFC3339, s.Timestamp)
		if err != nil {
			continue
		}

		if ts.After(cutoff) {
			kept = append(kept, s)
		} else {
			// Delete snapshot file
			os.Remove(filepath.Join(snapshotsDir, s.CommitSHA+".json"))
		}
	}

	index.Snapshots = kept
	saveIndex(index)
}

// =============================================================================
// Subcommands (stubs)
// gl:docs/spec/git-glfix/incremental-tracking.md#L148
// =============================================================================

func runCacheCommand(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: git-glfix cache <list|show|prune|clear|rebuild>")
		return
	}

	switch args[0] {
	case "list":
		index, err := loadIndex()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return
		}
		fmt.Printf("Snapshots (%d):\n", len(index.Snapshots))
		for _, s := range index.Snapshots {
			fmt.Printf("  %s %s %s %v\n", s.CommitSHA[:8], s.Branch, s.Timestamp, s.Tags)
		}

	case "show":
		if len(args) < 2 {
			fmt.Println("Usage: git-glfix cache show <commit>")
			return
		}
		snapshot, err := loadSnapshot(args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(snapshot)

	case "prune":
		config := loadConfig()
		pruneSnapshots(config)
		fmt.Println("Pruning complete")

	case "clear":
		os.RemoveAll(glLinksDir)
		fmt.Println("Cache cleared")

	case "rebuild":
		os.RemoveAll(glLinksDir)
		if err := run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}

	default:
		fmt.Printf("Unknown cache command: %s\n", args[0])
	}
}

func runConfigCommand(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: git-glfix config <list|set|unset>")
		return
	}

	switch args[0] {
	case "list":
		out, _ := exec.Command("git", "config", "--get-regexp", "gl-links").Output()
		if len(out) == 0 {
			fmt.Println("No gl-links configuration set")
		} else {
			fmt.Print(string(out))
		}

	case "set":
		if len(args) < 3 {
			fmt.Println("Usage: git-glfix config set <key> <value>")
			return
		}
		key := "gl-links." + args[1]
		exec.Command("git", "config", key, args[2]).Run()
		fmt.Printf("Set %s = %s\n", key, args[2])

	case "unset":
		if len(args) < 2 {
			fmt.Println("Usage: git-glfix config unset <key>")
			return
		}
		key := "gl-links." + args[1]
		exec.Command("git", "config", "--unset", key).Run()
		fmt.Printf("Unset %s\n", key)

	default:
		fmt.Printf("Unknown config command: %s\n", args[0])
	}
}

func runStatusCommand() {
	head, _ := getHeadCommit()
	branch, _ := getCurrentBranch()
	if branch == "" {
		branch = "HEAD (detached)"
	}

	fmt.Printf("Branch: %s\n", branch)
	fmt.Printf("HEAD: %s\n", head[:8])

	snapshot, err := selectBestSnapshot(head, branch)
	if err != nil {
		fmt.Printf("Snapshot: none\n")
	} else {
		commits, _ := getCommitsBetween(snapshot.CommitSHA, head)
		fmt.Printf("Snapshot: %s (%d links, %d commits behind)\n",
			snapshot.CommitSHA[:8], len(snapshot.Links), len(commits))
	}

	links, _ := discoverLinks()
	fmt.Printf("Links in worktree: %d\n", len(links))
}

func runValidateCommand() {
	links, err := discoverLinks()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}

	var valid, broken int
	for _, link := range links {
		if _, err := os.Stat(link.TargetFile); os.IsNotExist(err) {
			fmt.Printf("BROKEN: %s:%d -> %s (file not found)\n",
				link.SourceFile, link.SourceLine, link.TargetFile)
			broken++
			continue
		}

		content, err := os.ReadFile(link.TargetFile)
		if err != nil {
			fmt.Printf("BROKEN: %s:%d -> %s (cannot read)\n",
				link.SourceFile, link.SourceLine, link.TargetFile)
			broken++
			continue
		}

		lines := strings.Split(string(content), "\n")

		// Validate start line
		if link.TrackedLine <= 0 || link.TrackedLine > len(lines) {
			if link.TrackedEndLine > 0 {
				fmt.Printf("BROKEN: %s:%d -> %s#L%d-L%d (start line out of range, max %d)\n",
					link.SourceFile, link.SourceLine, link.TargetFile, link.TrackedLine, link.TrackedEndLine, len(lines))
			} else {
				fmt.Printf("BROKEN: %s:%d -> %s#L%d (line out of range, max %d)\n",
					link.SourceFile, link.SourceLine, link.TargetFile, link.TrackedLine, len(lines))
			}
			broken++
			continue
		}

		// Validate end line if this is a range
		if link.TrackedEndLine > 0 {
			if link.TrackedEndLine <= 0 || link.TrackedEndLine > len(lines) {
				fmt.Printf("BROKEN: %s:%d -> %s#L%d-L%d (end line out of range, max %d)\n",
					link.SourceFile, link.SourceLine, link.TargetFile, link.TrackedLine, link.TrackedEndLine, len(lines))
				broken++
				continue
			}
			// Validate range is sensible (start <= end)
			if link.TrackedLine > link.TrackedEndLine {
				fmt.Printf("BROKEN: %s:%d -> %s#L%d-L%d (invalid range: start > end)\n",
					link.SourceFile, link.SourceLine, link.TargetFile, link.TrackedLine, link.TrackedEndLine)
				broken++
				continue
			}
		}

		valid++
	}

	fmt.Printf("\nValidation: %d valid, %d broken\n", valid, broken)
}
