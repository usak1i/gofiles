package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func put(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func contentIs(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s: got %q, want %q", path, got, want)
	}
}

func runCLI(args ...string) (string, error) {
	cmd := newRootCmd()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return output.String(), err
}

func TestCategories(t *testing.T) {
	groups := map[string]string{
		"Documents": "pdf txt md doc docx xls xlsx ppt pptx csv",
		"Images":    "jpg jpeg png gif webp svg heic",
		"Archives":  "zip rar 7z tar gz bz2 xz",
		"Audio":     "mp3 wav flac m4a aac ogg",
		"Video":     "mp4 mov mkv avi webm",
	}
	for want, extensions := range groups {
		for _, ext := range strings.Fields(extensions) {
			for _, name := range []string{"file." + ext, "file." + strings.ToUpper(ext)} {
				if got := category(name); got != want {
					t.Errorf("%s: got %s, want %s", name, got, want)
				}
			}
		}
	}
	for _, name := range []string{"README", "file.unknown", "file."} {
		if got := category(name); got != "Others" {
			t.Errorf("%s: got %s", name, got)
		}
	}
}

func TestPreviewApplyAndRepeat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "下載 files")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"報告 one.PDF": "Documents", "photo.png": "Images", "backup.tar.gz": "Archives",
		"song.mp3": "Audio", "movie.mov": "Video", "README": "Others", "data.xyz": "Others",
	}
	for name := range files {
		put(t, filepath.Join(dir, name), name)
	}
	put(t, filepath.Join(dir, ".hidden.txt"), "hidden")
	nested := filepath.Join(dir, "nested")
	if err := os.Mkdir(nested, 0755); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(nested, "inside.txt"), "nested")
	link := filepath.Join(dir, "link.pdf")
	if err := os.Symlink("報告 one.PDF", link); err != nil {
		t.Fatal(err)
	}
	output, err := runCLI("organize", dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "7 planned, 3 skipped, 0 failed") {
		t.Fatal(output)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 10 {
		t.Fatalf("preview changed directory: %d entries", len(entries))
	}
	for name := range files {
		contentIs(t, filepath.Join(dir, name), name)
	}
	output, err = runCLI("organize", dir, "--apply")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "Moved: 7, skipped: 3, failed: 0") {
		t.Fatal(output)
	}
	for name, group := range files {
		contentIs(t, filepath.Join(dir, group, name), name)
		if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("original still exists: %s (%v)", name, err)
		}
	}
	contentIs(t, filepath.Join(dir, ".hidden.txt"), "hidden")
	contentIs(t, filepath.Join(nested, "inside.txt"), "nested")
	if target, err := os.Readlink(link); err != nil || target != "報告 one.PDF" {
		t.Fatalf("link changed: %q, %v", target, err)
	}
	output, err = runCLI("organize", dir, "--apply")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "Moved: 0") {
		t.Fatal(output)
	}
}

func TestConflicts(t *testing.T) {
	for _, kind := range []string{"file", "directory", "dangling symlink"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join(dir, "a.pdf")
			put(t, source, "original")
			if err := os.Mkdir(filepath.Join(dir, "Documents"), 0755); err != nil {
				t.Fatal(err)
			}
			destination := filepath.Join(dir, "Documents", "a.pdf")
			switch kind {
			case "file":
				put(t, destination, "existing")
			case "directory":
				if err := os.Mkdir(destination, 0755); err != nil {
					t.Fatal(err)
				}
			case "dangling symlink":
				if err := os.Symlink("missing", destination); err != nil {
					t.Fatal(err)
				}
			}
			for _, apply := range []bool{false, true} {
				var output bytes.Buffer
				if err := organize(dir, apply, &output); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(output.String(), "destination exists") {
					t.Fatal(output.String())
				}
				contentIs(t, source, "original")
			}
			if kind == "file" {
				contentIs(t, destination, "existing")
			}
		})
	}
}

func TestInvalidCategoryContinues(t *testing.T) {
	for _, symlink := range []bool{false, true} {
		t.Run(map[bool]string{false: "file", true: "symlink"}[symlink], func(t *testing.T) {
			dir := t.TempDir()
			outside := t.TempDir()
			categoryPath := filepath.Join(dir, "Documents")
			if symlink {
				if err := os.Symlink(outside, categoryPath); err != nil {
					t.Fatal(err)
				}
			} else {
				put(t, categoryPath, "blocking file")
			}
			put(t, filepath.Join(dir, "a.pdf"), "keep")
			put(t, filepath.Join(dir, "z.png"), "move")
			// Preview must identify the failure without creating any output directories.
			if _, err := runCLI("organize", dir); err == nil {
				t.Fatal("expected preview error")
			}
			if _, err := os.Stat(filepath.Join(dir, "Images")); !os.IsNotExist(err) {
				t.Fatal("preview created directory")
			}
			output, err := runCLI("organize", dir, "--apply")
			if err == nil {
				t.Fatal("expected apply error")
			}
			if !strings.Contains(output, "failed: 1") {
				t.Fatal(output)
			}
			contentIs(t, filepath.Join(dir, "a.pdf"), "keep")
			contentIs(t, filepath.Join(dir, "Images", "z.png"), "move")
			entries, err := os.ReadDir(outside)
			if err != nil || len(entries) != 0 {
				t.Fatalf("outside directory changed: %v", err)
			}
		})
	}
}

func TestMoveRefusesExistingDestination(t *testing.T) {
	dir := t.TempDir()
	source, destination := filepath.Join(dir, "source"), filepath.Join(dir, "destination")
	put(t, source, "original")
	put(t, destination, "existing")
	if err := moveWithoutOverwrite(source, destination); !os.IsExist(err) {
		t.Fatalf("expected exists error: %v", err)
	}
	contentIs(t, source, "original")
	contentIs(t, destination, "existing")
}

func TestInvalidArguments(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	put(t, file, "data")
	for _, args := range [][]string{
		{"organize"}, {"organize", "a", "b"}, {"organize", file}, {"organize", filepath.Join(t.TempDir(), "missing")},
	} {
		if _, err := runCLI(args...); err == nil {
			t.Errorf("expected error for %v", args)
		}
	}
}
