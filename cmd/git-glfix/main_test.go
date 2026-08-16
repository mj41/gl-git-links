package main

import "testing"

// Test data never contains a literal "gl:" prefix — it is assembled from `gl`
// below instead. This tool scans every tracked file for links, test files
// included, so an example link written out in full becomes a real link in this
// repository: it shows up in `validate` as broken forever (its target does not
// exist), and if the target DID exist the tool would rewrite the test's own
// expectations when lines moved. Fixtures that the fixture-checker mistakes for
// production data is a trap worth avoiding by construction.
const gl = "gl" + ":"

// The link grammar and the hunk arithmetic are the two pieces of this tool that
// are pure functions of their input, and until now neither had a test: every
// assertion lived in git-glfix-test, which needs a git repository with a
// hand-built history and covers them only through whatever ../gl-exA happens to
// contain. Nothing there exercises a range at all.
//
// The cases below are written against docs/spec/gl-spec.md, so a divergence
// shows up here as a failure naming the section it violates.

// Link syntax, per the fragment grammar in gl-spec.md §2.4:
//
//	#L[1-9][0-9]*(-L[1-9][0-9]*)?
func TestLinkGrammar(t *testing.T) {
	cases := []struct {
		name       string
		in         string
		path       string
		start, end string // "" for no match / no end
	}{
		{"single line", "see " + gl + "docs/a.md#L42 here", "docs/a.md", "42", ""},
		{"range", "see " + gl + "docs/a.md#L42-L58 here", "docs/a.md", "42", "58"},
		{"relative path", gl + "./sib.md#L7", "./sib.md", "7", ""},
		{"go comment", "// " + gl + "pkg/x.go#L120 - why", "pkg/x.go", "120", ""},

		// §3.2.2: trailing punctuation is not part of the link. The digits stop
		// at the first non-digit, so these need no special casing — but they are
		// the cases a future regex change would break first.
		{"trailing period", "at " + gl + "a.md#L9.", "a.md", "9", ""},
		{"trailing comma", "at " + gl + "a.md#L9, and", "a.md", "9", ""},
		{"in parentheses", "(" + gl + "a.md#L9)", "a.md", "9", ""},
		{"angle brackets", "<" + gl + "a.md#L42>", "a.md", "42", ""},
		{"range then period", gl + "a.md#L4-L9.", "a.md", "4", "9"},

		// §2.4: a fragment that is not a line spec is not a line link. Before
		// this was pinned, #L0 was tracked as line zero and reported broken
		// forever, and #L007 was read as line 7 and then rewritten in place.
		{"zero is not a line", gl + "a.md#L0", "", "", ""},
		{"leading zero", gl + "a.md#L007", "", "", ""},
		{"zero end of range", gl + "a.md#L4-L0", "a.md", "4", ""},
		{"lowercase l", gl + "a.md#l42", "", "", ""},
		{"no fragment is not tracked", gl + "a.md and text", "", "", ""},
		{"empty fragment", gl + "a.md#L", "", "", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := linkRe.FindStringSubmatch(c.in)
			if c.path == "" {
				if m != nil {
					t.Fatalf("%q matched as path=%q start=%q; the spec does not make this a line link",
						c.in, m[1], m[2])
				}
				return
			}
			if m == nil {
				t.Fatalf("%q did not match, expected path=%q start=%q", c.in, c.path, c.start)
			}
			if m[1] != c.path || m[2] != c.start || m[3] != c.end {
				t.Errorf("%q -> path=%q start=%q end=%q, want path=%q start=%q end=%q",
					c.in, m[1], m[2], m[3], c.path, c.start, c.end)
			}
		})
	}
}

func TestLinkGrammarFindsEveryLinkOnALine(t *testing.T) {
	// A doc table puts several links on one line; discoverLinks relies on
	// FindAllStringSubmatch returning each of them.
	line := "| Link discovery | " + gl + "docs/spec.md#L26 | " + gl + "cmd/main.go#L741-L760 |"
	got := linkRe.FindAllStringSubmatch(line, -1)
	if len(got) != 2 {
		t.Fatalf("found %d links on the line, want 2", len(got))
	}
	if got[0][1] != "docs/spec.md" || got[1][3] != "760" {
		t.Errorf("wrong captures: %v", got)
	}
}

func TestParseHunkHeader(t *testing.T) {
	cases := []struct {
		in                                     string
		want                                   bool
		oldStart, oldCount, newStart, newCount int
	}{
		{"@@ -1,3 +1,4 @@", true, 1, 3, 1, 4},
		{"@@ -10 +10,2 @@", true, 10, 1, 10, 2},        // omitted count means 1
		{"@@ -0,0 +1,5 @@ func x()", true, 0, 0, 1, 5}, // pure insertion, with context
		{"not a hunk", false, 0, 0, 0, 0},
	}

	for _, c := range cases {
		got := parseHunkHeader(c.in)
		if !c.want {
			if got != nil {
				t.Errorf("%q parsed as %+v, want nil", c.in, got)
			}
			continue
		}
		if got == nil {
			t.Fatalf("%q did not parse", c.in)
		}
		if got.OldStart != c.oldStart || got.OldCount != c.oldCount ||
			got.NewStart != c.newStart || got.NewCount != c.newCount {
			t.Errorf("%q -> %+v, want -%d,%d +%d,%d",
				c.in, got, c.oldStart, c.oldCount, c.newStart, c.newCount)
		}
	}
}

// applyHunksToLine is where a link's new line number actually comes from.
// -1 means the line itself was deleted, which is what surfaces as "broken".
func TestApplyHunksToLine(t *testing.T) {
	insertTwoAtTop := []Hunk{{OldStart: 1, OldCount: 0, NewStart: 1, NewCount: 2}}
	deleteLines2to3 := []Hunk{{OldStart: 2, OldCount: 2, NewStart: 2, NewCount: 0}}

	cases := []struct {
		name  string
		line  int
		hunks []Hunk
		want  int
	}{
		{"no hunks leaves the line alone", 42, nil, 42},
		{"insertion above shifts down", 5, insertTwoAtTop, 7},
		{"insertion below does not move it", 1, []Hunk{{OldStart: 9, OldCount: 0, NewStart: 9, NewCount: 3}}, 1},
		{"deleted line reports -1", 2, deleteLines2to3, -1},
		{"line after a deletion shifts up", 5, deleteLines2to3, 3},
		{"two hunks accumulate", 20, []Hunk{
			{OldStart: 1, OldCount: 0, NewStart: 1, NewCount: 2},
			{OldStart: 10, OldCount: 4, NewStart: 12, NewCount: 1},
		}, 19}, // +2 then -3
	}

	for _, c := range cases {
		if got := applyHunksToLine(c.line, c.hunks); got != c.want {
			t.Errorf("%s: line %d -> %d, want %d", c.name, c.line, got, c.want)
		}
	}
}

// A range is two independent line numbers, which is the whole of the feature:
// the start and the end move separately, so the range can widen and narrow.
func TestRangeEndsMoveIndependently(t *testing.T) {
	// Three lines inserted between line 4 and line 9 — inside the range.
	insertInside := []Hunk{{OldStart: 6, OldCount: 0, NewStart: 6, NewCount: 3}}

	start := applyHunksToLine(4, insertInside)
	end := applyHunksToLine(9, insertInside)

	if start != 4 {
		t.Errorf("start moved to %d; an insertion below the start must not move it", start)
	}
	if end != 12 {
		t.Errorf("end moved to %d, want 12; the range must widen by the 3 inserted lines", end)
	}
	if end-start != 8 {
		t.Errorf("range spans %d lines, want 8", end-start)
	}
}
