package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
)

// Version is the semantic version of git-glfix
const Version = "0.1.0"

// BuildInfo contains version and build information
type BuildInfo struct {
	Version   string
	GitCommit string
	GitDirty  bool
	BuildTime string
	GoVersion string
	Module    string
}

// GetBuildInfo returns build information using debug.ReadBuildInfo()
func GetBuildInfo() BuildInfo {
	info := BuildInfo{
		Version: Version,
	}

	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		info.GoVersion = buildInfo.GoVersion
		info.Module = buildInfo.Main.Path

		// Parse VCS information from build settings
		for _, setting := range buildInfo.Settings {
			switch setting.Key {
			case "vcs.revision":
				info.GitCommit = setting.Value
				if len(info.GitCommit) > 12 {
					info.GitCommit = info.GitCommit[:12] // Shorten to 12 chars
				}
			case "vcs.modified":
				info.GitDirty = setting.Value == "true"
			case "vcs.time":
				info.BuildTime = setting.Value
			}
		}
	}

	return info
}

// FormatVersion returns a formatted version string for git-glfix
func FormatVersion() string {
	buildInfo := GetBuildInfo()

	var lines []string

	// Main version line
	versionLine := fmt.Sprintf("git-glfix version %s", buildInfo.Version)
	if buildInfo.GitCommit != "" {
		commit := buildInfo.GitCommit
		if buildInfo.GitDirty {
			versionLine += fmt.Sprintf(" (commit %s, dirty)", commit)
		} else {
			versionLine += fmt.Sprintf(" (commit %s)", commit)
		}
	}
	lines = append(lines, versionLine)

	// Build time
	if buildInfo.BuildTime != "" {
		lines = append(lines, fmt.Sprintf("Built: %s", buildInfo.BuildTime))
	}

	// Go version
	if buildInfo.GoVersion != "" {
		lines = append(lines, fmt.Sprintf("Go Version: %s", buildInfo.GoVersion))
	}

	// OS/Arch
	lines = append(lines, fmt.Sprintf("OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))

	return strings.Join(lines, "\n")
}

// ShowVersion prints version information and exits
func ShowVersion() {
	fmt.Println(FormatVersion())
}
