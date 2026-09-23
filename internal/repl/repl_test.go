package repl

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runScript(t *testing.T, script string) string {
	t.Helper()
	var out bytes.Buffer
	if err := Run(strings.NewReader(script), &out); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return out.String()
}

func TestHelpExitAndUnknownCommand(t *testing.T) {
	out := runScript(t, "help\nnope\ndependents c12\nexit\n")
	if !strings.Contains(out, "build <folder>") {
		t.Fatalf("help missing from output:\n%s", out)
	}
	if !strings.Contains(out, `unknown command "nope"`) {
		t.Fatalf("unknown command was not reported:\n%s", out)
	}
	if !strings.Contains(out, "run build before dependents") {
		t.Fatalf("dependents before build should fail:\n%s", out)
	}
	if strings.Count(out, "navdep> ") < 4 {
		t.Fatalf("session ended before later commands:\n%s", out)
	}
}

func TestQuitAndEOF(t *testing.T) {
	out := runScript(t, "quit\nhelp\n")
	if strings.Contains(out, "build <folder>") {
		t.Fatalf("quit should stop before help:\n%s", out)
	}

	var buf bytes.Buffer
	if err := Run(strings.NewReader(""), &buf); err != nil {
		t.Fatal(err)
	}
	if buf.String() != prompt {
		t.Fatalf("EOF output = %q", buf.String())
	}
}

func TestBuildReplacesMapAndKeepsItOnFailure(t *testing.T) {
	dir := t.TempDir()
	spaced := filepath.Join(t.TempDir(), "My Objects")
	if err := os.Mkdir(spaced, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "not-a-folder.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	script := strings.Join([]string{
		`build`,
		`build "` + dir + `"`,
		`dependents C12`,
		`build "` + filepath.Join(dir, "missing") + `"`,
		`dependents c12`,
		`build "` + file + `"`,
		`build "` + spaced + `"`,
		`dependents nope`,
		`exit extra`,
		`exit`,
		"",
	}, "\n")
	out := runScript(t, script)

	if !strings.Contains(out, "error: build requires a folder") {
		t.Fatalf("missing folder arg:\n%s", out)
	}
	if strings.Count(out, "objects: 0, links: 0, unresolved: 0") != 2 {
		t.Fatalf("expected two successful builds (second replaces the map):\n%s", out)
	}
	if !strings.Contains(out, "folder not found:") {
		t.Fatalf("missing folder should be reported:\n%s", out)
	}
	if !strings.Contains(out, "not a folder:") {
		t.Fatalf("file path should be rejected:\n%s", out)
	}
	if strings.Contains(out, "run build before dependents") {
		t.Fatalf("failed rebuild should leave the previous map:\n%s", out)
	}
	if !strings.Contains(out, `invalid object key "nope"`) {
		t.Fatalf("invalid key:\n%s", out)
	}
	if !strings.Contains(out, "exit takes no arguments") {
		t.Fatalf("exit with args should not leave the session:\n%s", out)
	}
}

func TestQuotedPathAndBlankLines(t *testing.T) {
	out := runScript(t, "\n\"unterminated\nhelp extra\nquit\n")
	if !strings.Contains(out, "unterminated quote") {
		t.Fatalf("quote error missing:\n%s", out)
	}
	if !strings.Contains(out, "help takes no arguments") {
		t.Fatalf("help args missing:\n%s", out)
	}
}
