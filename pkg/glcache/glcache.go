package glcache

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// =============================================================================
// Data Structures
// =============================================================================

// SnapshotIndex is the registry of all snapshots
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
type Snapshot struct {
	CommitSHA string `json:"commit_sha"`
	Branch    string `json:"branch"`
	Timestamp string `json:"timestamp"`
	Links     []Link `json:"links"`
}

// Link represents a gl: link instance
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
type History struct {
	OriginCommit  string         `json:"origin_commit"`
	OriginLine    int            `json:"origin_line"`
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

// =============================================================================
// Configuration
// =============================================================================

const (
	glLinksDir   = ".git/gl-links"
	snapshotsDir = ".git/gl-links/snapshots"
	indexFile    = ".git/gl-links/index.json"
	indexVersion = 1
)

// DiscoverOptions controls link discovery behavior
type DiscoverOptions struct {
	IncludeUntracked bool // Include untracked files in discovery
}

// LinkRe matches a gl: link carrying a line spec: path, start line, optional
// end line. It is exported so there is exactly ONE grammar in this module —
// cmd/git-glfix used to keep a second copy, and the two had drifted apart:
// this one had no range group at all, so the library reported a range as a
// single-line link, and both accepted "#L0" and "#L007", which
// docs/spec/gl-spec.md excludes by requiring a non-zero first digit.
//
// The line spec is required. A link without one has no line to track, and
// nothing in this module should invent one for it.
var LinkRe = regexp.MustCompile(`gl:(\S+?)#L([1-9][0-9]*)(?:-L([1-9][0-9]*))?`)

// =============================================================================
// Public API - Cache Operations
// =============================================================================

// LoadSnapshot loads a specific snapshot by commit SHA
func LoadSnapshot(commitSHA string) (*Snapshot, error) {
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

// SaveSnapshot saves a snapshot for a commit
func SaveSnapshot(commitSHA, branch string, links []Link) error {
	if err := ensureStorageDir(); err != nil {
		return err
	}

	snapshot := Snapshot{
		CommitSHA: commitSHA,
		Branch:    branch,
		Timestamp: currentTimestamp(),
		Links:     links,
	}

	path := filepath.Join(snapshotsDir, commitSHA+".json")
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}

	// Update index
	index, err := LoadIndex()
	if err != nil {
		return err
	}

	// Remove old entry if exists
	for i, s := range index.Snapshots {
		if s.CommitSHA == commitSHA {
			index.Snapshots = append(index.Snapshots[:i], index.Snapshots[i+1:]...)
			break
		}
	}

	// Add new entry
	index.Snapshots = append(index.Snapshots, SnapshotEntry{
		CommitSHA: commitSHA,
		Branch:    branch,
		Timestamp: snapshot.Timestamp,
	})

	return saveIndex(index)
}

// LoadIndex loads the snapshot index
func LoadIndex() (*SnapshotIndex, error) {
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

// LoadLatestSnapshot finds and loads the best snapshot for current HEAD
func LoadLatestSnapshot() (*Snapshot, error) {
	head, err := GetHeadCommit()
	if err != nil {
		return nil, err
	}

	branch, _ := GetCurrentBranch()

	index, err := LoadIndex()
	if err != nil {
		return nil, err
	}

	var best *SnapshotEntry
	for i := range index.Snapshots {
		s := &index.Snapshots[i]
		if s.Branch != branch && branch != "" {
			continue
		}
		if !IsAncestor(s.CommitSHA, head) {
			continue
		}
		if best == nil {
			best = s
		} else {
			// Prefer more recent snapshots (later in commits)
			if IsAncestor(best.CommitSHA, s.CommitSHA) {
				best = s
			}
		}
	}

	if best == nil {
		return nil, fmt.Errorf("no valid snapshot found")
	}

	return LoadSnapshot(best.CommitSHA)
}

// ClearCache removes all snapshots
func ClearCache() error {
	return os.RemoveAll(glLinksDir)
}

// =============================================================================
// Public API - Link Discovery
// =============================================================================

// DiscoverLinks finds all gl: links in the repository
func DiscoverLinks(opts DiscoverOptions) ([]Link, error) {
	files, err := getFiles(opts.IncludeUntracked)
	if err != nil {
		return nil, err
	}

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
			for _, match := range LinkRe.FindAllStringSubmatch(line, -1) {
				targetPathRaw := match[1]
				targetLine, _ := strconv.Atoi(match[2])

				// Ranges were dropped here entirely: the old pattern had no
				// second group, so a link written #L4-L9 came back as a plain
				// link to line 4 and the range vanished without a word. Consumers
				// building back-references got a link that pointed somewhere
				// narrower than the author wrote.
				var targetEndLine int
				if match[3] != "" {
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

// QueryLinksByTarget returns all links pointing to a specific target file
func QueryLinksByTarget(targetFile string, opts DiscoverOptions) ([]Link, error) {
	allLinks, err := DiscoverLinks(opts)
	if err != nil {
		return nil, err
	}

	var result []Link
	for _, link := range allLinks {
		if link.TargetFile == targetFile {
			result = append(result, link)
		}
	}

	return result, nil
}

// GroupLinksByTarget groups links by their target file
func GroupLinksByTarget(links []Link) map[string][]Link {
	result := make(map[string][]Link)
	for _, link := range links {
		result[link.TargetFile] = append(result[link.TargetFile], link)
	}
	return result
}

// GroupLinksByTargetLine groups links by target file and line number
func GroupLinksByTargetLine(links []Link) map[string]map[int][]Link {
	result := make(map[string]map[int][]Link)
	for _, link := range links {
		if result[link.TargetFile] == nil {
			result[link.TargetFile] = make(map[int][]Link)
		}
		result[link.TargetFile][link.TrackedLine] = append(result[link.TargetFile][link.TrackedLine], link)
	}
	return result
}

// =============================================================================
// Public API - Git Helpers
// =============================================================================

// GetHeadCommit returns current HEAD commit SHA
func GetHeadCommit() (string, error) {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// GetCurrentBranch returns current branch name
func GetCurrentBranch() (string, error) {
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

// IsAncestor checks if ancestor is an ancestor of descendant
func IsAncestor(ancestor, descendant string) bool {
	err := exec.Command("git", "merge-base", "--is-ancestor", ancestor, descendant).Run()
	return err == nil
}

// GetRepoRoot returns the git repository root path
func GetRepoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// =============================================================================
// Internal Helpers
// =============================================================================

func ensureStorageDir() error {
	return os.MkdirAll(snapshotsDir, 0755)
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

func currentTimestamp() string {
	return time.Now().Format(time.RFC3339)
}

func getFiles(includeUntracked bool) ([]string, error) {
	if includeUntracked {
		// Get all files recursively
		return getAllFiles()
	}
	// Get only tracked files
	out, err := exec.Command("git", "ls-files").Output()
	if err != nil {
		return nil, err
	}
	return parseLines(out), nil
}

func getAllFiles() ([]string, error) {
	var files []string
	root, err := GetRepoRoot()
	if err != nil {
		return nil, err
	}

	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Skip .git directory
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		// Make path relative to root
		relPath, _ := filepath.Rel(root, path)
		files = append(files, relPath)
		return nil
	})

	return files, err
}

func parseLines(output []byte) []string {
	text := strings.TrimSpace(string(output))
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
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

// ComputeLineHash computes MD5 hash of trimmed line content
func ComputeLineHash(content string) string {
	trimmed := strings.TrimSpace(content)
	hash := md5.Sum([]byte(trimmed))
	return hex.EncodeToString(hash[:])
}
