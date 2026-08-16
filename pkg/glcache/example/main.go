package main

import (
	"fmt"
	"log"

	"github.com/mj41/gl-git-links/pkg/glcache"
)

// Example: Using glcache to find backlinks for documentation
func main() {
	// Example 1: Discover all gl: links in the repository
	fmt.Println("=== Example 1: Discover All Links ===")
	links, err := glcache.DiscoverLinks(glcache.DiscoverOptions{
		IncludeUntracked: true, // Include uncommitted files
	})
	if err != nil {
		log.Fatalf("Error discovering links: %v", err)
	}
	fmt.Printf("Found %d gl: links\n\n", len(links))

	// Example 2: Find all links pointing to a specific documentation file
	fmt.Println("=== Example 2: Query Links by Target (Backlinks) ===")
	targetFile := "docs/dev/git-glfix/readme.md"
	backlinks, err := glcache.QueryLinksByTarget(targetFile, glcache.DiscoverOptions{
		IncludeUntracked: true,
	})
	if err != nil {
		log.Fatalf("Error querying links: %v", err)
	}

	fmt.Printf("Backlinks to %s:\n", targetFile)
	for _, link := range backlinks {
		fmt.Printf("  %s:%d -> #L%d\n",
			link.SourceFile, link.SourceLine, link.TrackedLine)
	}
	fmt.Println()

	// Example 3: Group links by target file
	fmt.Println("=== Example 3: Group Links by Target ===")
	grouped := glcache.GroupLinksByTarget(links)
	for target, targetLinks := range grouped {
		fmt.Printf("%s: %d links\n", target, len(targetLinks))
	}
	fmt.Println()

	// Example 4: Group links by target file and line (for section-level backlinks)
	fmt.Println("=== Example 4: Group by Target and Line ===")
	byLine := glcache.GroupLinksByTargetLine(backlinks)
	for file, lineMap := range byLine {
		fmt.Printf("%s:\n", file)
		for line, lineLinks := range lineMap {
			fmt.Printf("  Line %d: %d backlinks\n", line, len(lineLinks))
			for _, link := range lineLinks {
				fmt.Printf("    - %s:%d\n", link.SourceFile, link.SourceLine)
			}
		}
	}
	fmt.Println()

	// Example 5: Check if cache exists and load latest snapshot
	fmt.Println("=== Example 5: Load Cache Snapshot ===")
	snapshot, err := glcache.LoadLatestSnapshot()
	if err != nil {
		fmt.Printf("No snapshot available: %v\n", err)
		fmt.Println("Run git-glfix to create a snapshot first")
	} else {
		fmt.Printf("Latest snapshot: commit %s\n", snapshot.CommitSHA[:8])
		fmt.Printf("Tracked links: %d\n", len(snapshot.Links))
	}
	fmt.Println()

	// Example 6: Git repository info
	fmt.Println("=== Example 6: Git Info ===")
	head, _ := glcache.GetHeadCommit()
	branch, _ := glcache.GetCurrentBranch()
	root, _ := glcache.GetRepoRoot()
	fmt.Printf("Repository: %s\n", root)
	fmt.Printf("Branch: %s\n", branch)
	fmt.Printf("HEAD: %s\n", head[:8])
}
