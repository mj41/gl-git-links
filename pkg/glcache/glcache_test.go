package glcache_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mj41/gl-git-links/pkg/glcache"
)

// Test data never contains a literal "gl:" prefix — it is assembled from `gl`
// below instead. This tool scans every tracked file for links, test files
// included, so an example link written out in full becomes a real link in this
// repository: it shows up in `validate` as broken forever (its target does not
// exist), and if the target DID exist the tool would rewrite the test's own
// expectations when lines moved. Fixtures that the fixture-checker mistakes for
// production data is a trap worth avoiding by construction.
const gl = "gl" + ":"

// Test basic operations without requiring a git repository
func TestDiscoverOptions(t *testing.T) {
	opts := glcache.DiscoverOptions{
		IncludeUntracked: true,
	}

	if !opts.IncludeUntracked {
		t.Error("IncludeUntracked should be true")
	}
}

func TestGroupLinksByTarget(t *testing.T) {
	links := []glcache.Link{
		{ID: 1, SourceFile: "a.md", TargetFile: "target1.md", TrackedLine: 10},
		{ID: 2, SourceFile: "b.md", TargetFile: "target1.md", TrackedLine: 20},
		{ID: 3, SourceFile: "c.md", TargetFile: "target2.md", TrackedLine: 30},
	}

	grouped := glcache.GroupLinksByTarget(links)

	if len(grouped) != 2 {
		t.Errorf("Expected 2 target files, got %d", len(grouped))
	}

	if len(grouped["target1.md"]) != 2 {
		t.Errorf("Expected 2 links to target1.md, got %d", len(grouped["target1.md"]))
	}

	if len(grouped["target2.md"]) != 1 {
		t.Errorf("Expected 1 link to target2.md, got %d", len(grouped["target2.md"]))
	}
}

func TestGroupLinksByTargetLine(t *testing.T) {
	links := []glcache.Link{
		{ID: 1, SourceFile: "a.md", TargetFile: "target1.md", TrackedLine: 10},
		{ID: 2, SourceFile: "b.md", TargetFile: "target1.md", TrackedLine: 10},
		{ID: 3, SourceFile: "c.md", TargetFile: "target1.md", TrackedLine: 20},
	}

	grouped := glcache.GroupLinksByTargetLine(links)

	if len(grouped) != 1 {
		t.Errorf("Expected 1 target file, got %d", len(grouped))
	}

	if len(grouped["target1.md"]) != 2 {
		t.Errorf("Expected 2 line numbers, got %d", len(grouped["target1.md"]))
	}

	if len(grouped["target1.md"][10]) != 2 {
		t.Errorf("Expected 2 links to line 10, got %d", len(grouped["target1.md"][10]))
	}

	if len(grouped["target1.md"][20]) != 1 {
		t.Errorf("Expected 1 link to line 20, got %d", len(grouped["target1.md"][20]))
	}
}

func TestComputeLineHash(t *testing.T) {
	content := "  hello world  "
	hash := glcache.ComputeLineHash(content)

	if hash == "" {
		t.Error("Hash should not be empty")
	}

	// Same content with different whitespace should produce same hash
	content2 := "hello world"
	hash2 := glcache.ComputeLineHash(content2)

	if hash != hash2 {
		t.Error("Hashes should match for trimmed content")
	}
}

// The library kept its own link pattern until the grammar moved here, and the
// two copies had quietly diverged: this one had no range group, so a link
// written as a range came back pointing at its first line only, and a consumer
// building back-references had no way to know. These pin the shared grammar
// from the library side; cmd/git-glfix/main_test.go pins the same rules from
// the tool's side, and both now read the same variable.
func TestLinkReMatchesTheSpecGrammar(t *testing.T) {
	cases := []struct {
		in               string
		path, start, end string
	}{
		{"see " + gl + "docs/a.md#L42 here", "docs/a.md", "42", ""},
		{"see " + gl + "docs/a.md#L42-L58 here", "docs/a.md", "42", "58"},
		{"// " + gl + "pkg/x.go#L7 - note", "pkg/x.go", "7", ""},
		{gl + "a.md#L9.", "a.md", "9", ""},
		{gl + "a.md#L0", "", "", ""},
		{gl + "a.md#L007", "", "", ""},
		{gl + "a.md with no fragment", "", "", ""},
	}

	for _, c := range cases {
		m := glcache.LinkRe.FindStringSubmatch(c.in)
		if c.path == "" {
			if m != nil {
				t.Errorf("%q matched (path=%q); the spec does not make this a line link", c.in, m[1])
			}
			continue
		}
		if m == nil {
			t.Errorf("%q did not match", c.in)
			continue
		}
		if m[1] != c.path || m[2] != c.start || m[3] != c.end {
			t.Errorf("%q -> %q/%q/%q, want %q/%q/%q", c.in, m[1], m[2], m[3], c.path, c.start, c.end)
		}
	}
}

// Link is what a snapshot round-trips through, and it is written by the CLI and
// read here. A field missing on this side is data silently dropped.
func TestLinkCarriesRangeEndThroughJSON(t *testing.T) {
	const written = `{"id":1,"source_file":"a.md","source_line":3,"target_file":"b.md",
	  "target_file_original":"b.md","tracked_line":4,"tracked_end_line":9,
	  "line_content_hash":"h","history":{"origin_commit":"c","origin_line":4}}`

	var l glcache.Link
	if err := json.Unmarshal([]byte(written), &l); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if l.TrackedEndLine != 9 {
		t.Errorf("tracked_end_line came back as %d, want 9 — a range read from a snapshot lost its end",
			l.TrackedEndLine)
	}

	out, err := json.Marshal(l)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(out), `"tracked_end_line":9`) {
		t.Errorf("re-marshalled without the end line: %s", out)
	}

	// A single-line link must not gain a spurious zero end.
	single := glcache.Link{ID: 2, TrackedLine: 5}
	out, _ = json.Marshal(single)
	if strings.Contains(string(out), "tracked_end_line") {
		t.Errorf("single-line link serialised an end line: %s", out)
	}
}
