package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDuplicateFilenameRule(t *testing.T) {
	for name, want := range map[string]bool{
		"report(1).pdf": true, "report (2).pdf": true, "照片 (123).png": true,
		"(3) notes.txt": true, "notes(1)edited.pdf": true, "file(1)(2).txt": true,
		"normal.pdf": false, "file(0).pdf": false, "file(-1).pdf": false,
		"file(abc).pdf": false, "file(1.5).pdf": false, "file（1）.pdf": false,
	} {
		if got := isDuplicateName(name); got != want {
			t.Errorf("%q: got %v, want %v", name, got, want)
		}
	}
}

func TestDuplicateScanAndCLI(t *testing.T) {
	dir := t.TempDir()
	put(t, filepath.Join(dir, "report (1).pdf"), "does not require matching contents or an original")
	put(t, filepath.Join(dir, "normal.pdf"), "normal")
	put(t, filepath.Join(dir, ".hidden (1)"), "hidden")
	if err := os.Mkdir(filepath.Join(dir, "folder (1)"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("normal.pdf", filepath.Join(dir, "link (1).pdf")); err != nil {
		t.Fatal(err)
	}
	plan, err := scanDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range plan.items {
		want := filepath.Base(item.source) == "report (1).pdf"
		if item.duplicate != want {
			t.Errorf("unexpected duplicate flag on %s", item.source)
		}
	}
	out, err := runCLI("organize", dir)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, "[DUP:") != 1 {
		t.Fatal(out)
	}
	contentIs(t, filepath.Join(dir, "report (1).pdf"), "does not require matching contents or an original")
}
