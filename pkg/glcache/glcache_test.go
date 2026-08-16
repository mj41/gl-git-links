package glcache_test

import (
	"testing"

	"github.com/mj41/gl-git-links/pkg/glcache"
)

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
