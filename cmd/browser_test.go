package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestBrowserStartupNavigationAndSelection(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "下載 files")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(dir, "keep.txt"), "parent")
	put(t, filepath.Join(child, "a.pdf"), "child")
	m := initialTUIModel(dir, false)
	if !m.browsing {
		t.Fatal("no-argument startup must browse")
	}
	m, _ = updateTUI(m, m.Init()())
	if len(m.plan.items) != 0 || m.selectedCount() != 0 {
		t.Fatal("browsing must not build an organize plan")
	}
	if len(m.folders) != 2 || m.folders[0].name != ".." || m.folders[1].path != child {
		t.Fatalf("unexpected folders: %+v", m.folders)
	}
	m, _ = press(m, tea.KeyDown)
	m, cmd := press(m, tea.KeyEnter)
	if !m.browsing || !m.loading {
		t.Fatal("enter should open the highlighted directory")
	}
	m, _ = updateTUI(m, cmd())
	if m.browserDir != child || !m.browserReady {
		t.Fatal("child directory not opened")
	}
	m, cmd = press(m, tea.KeyLeft)
	m, _ = updateTUI(m, cmd())
	if m.browserDir != dir {
		t.Fatal("left should return to parent")
	}
	m, _ = press(m, tea.KeyDown)
	m, cmd = press(m, tea.KeyRight)
	m, _ = updateTUI(m, cmd())
	m, cmd = press(m, 's')
	if m.browsing || !m.loading || cmd == nil {
		t.Fatal("s should open preview for current directory")
	}
	m, _ = updateTUI(m, cmd())
	if m.plan.directory != child || m.selectedCount() != 1 {
		t.Fatal("preview used wrong directory")
	}
	contentIs(t, filepath.Join(child, "a.pdf"), "child")
	contentIs(t, filepath.Join(dir, "keep.txt"), "parent")
	// Returning to folders must clear the previous selection when choosing again.
	m, cmd = press(m, 'b')
	m, _ = updateTUI(m, cmd())
	if !m.browsing || m.browserDir != child {
		t.Fatal("b should reopen browser at current folder")
	}
	m, cmd = press(m, tea.KeyLeft)
	m, _ = updateTUI(m, cmd())
	m, cmd = press(m, 's')
	m, _ = updateTUI(m, cmd())
	if m.plan.directory != dir || m.selectedCount() != 1 {
		t.Fatal("new folder must have a fresh plan")
	}
	m, _ = press(m, tea.KeyEnter)
	m, cmd = press(m, 'y')
	m = drainMoves(t, m, cmd)
	contentIs(t, filepath.Join(dir, "Documents", "keep.txt"), "parent")
	contentIs(t, filepath.Join(child, "a.pdf"), "child")
}

func TestBrowserEmptyDirectoryAndExplicitPath(t *testing.T) {
	dir := t.TempDir()
	m := initialTUIModel(dir, true)
	if m.browsing {
		t.Fatal("explicit path should retain direct preview")
	}
	m, _ = updateTUI(m, m.Init()())
	if len(m.plan.items) != 0 {
		t.Fatal("expected empty preview")
	}
	m, cmd := press(m, tea.KeyEscape)
	m, _ = updateTUI(m, cmd())
	if !m.browsing {
		t.Fatal("escape from preview should open browser")
	}
	m, cmd = press(m, 's')
	m, _ = updateTUI(m, cmd())
	if m.browsing || m.confirming || m.selectedCount() != 0 {
		t.Fatal("empty folder must be selectable without applying")
	}
}

func TestBrowserReadFailureKeepsPreviousDirectory(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "deleted")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatal(err)
	}
	m := initialTUIModel(dir, false)
	m, _ = updateTUI(m, m.Init()())
	m, _ = press(m, tea.KeyDown)
	if err := os.Remove(child); err != nil {
		t.Fatal(err)
	}
	m, cmd := press(m, tea.KeyEnter)
	m, _ = updateTUI(m, cmd())
	if m.loading || m.browserDir != dir || !strings.Contains(m.notice, "Cannot open folder") {
		t.Fatal("failed navigation must stay at prior directory")
	}
	m, cmd = press(m, 'r')
	m, _ = updateTUI(m, cmd())
	if len(m.folders) != 1 || m.exitErr != nil {
		t.Fatal("refresh should recover from browse error")
	}
	m, cmd = press(m, 's')
	m, _ = updateTUI(m, cmd())
	if m.browsing || m.plan.directory != dir {
		t.Fatal("valid parent should still be selectable")
	}
}

func TestBrowserUnreadableInitialDirectoryCanRecover(t *testing.T) {
	parent := t.TempDir()
	m := initialTUIModel(filepath.Join(parent, "missing"), false)
	m, _ = updateTUI(m, m.Init()())
	if m.browserReady {
		t.Fatal("unreadable directory cannot be selected")
	}
	m, cmd := press(m, 's')
	if cmd != nil || !m.browsing {
		t.Fatal("must not select an unreadable directory")
	}
	m, cmd = press(m, tea.KeyLeft)
	m, _ = updateTUI(m, cmd())
	if m.browserDir != parent || !m.browserReady {
		t.Fatal("parent navigation should recover")
	}
}

func TestBrowserFilteringRootAndSizing(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{".hidden", "中文", "z\x1b[31m\n"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	put(t, filepath.Join(dir, "file.txt"), "file")
	if err := os.Symlink(filepath.Join(dir, ".hidden"), filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	m := initialTUIModel(dir, false)
	m, _ = updateTUI(m, m.Init()())
	if len(m.folders) != 4 {
		t.Fatalf("expected parent and 3 real directories: %+v", m.folders)
	}
	for i := 0; i < 20; i++ {
		m, _ = press(m, tea.KeyDown)
	}
	if m.browserCursor != 3 {
		t.Fatal("cursor should clamp to last folder")
	}
	for _, size := range [][2]int{{100, 30}, {80, 24}, {35, 15}, {20, 5}, {1, 1}} {
		m, _ = updateTUI(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view := m.View().Content
		if strings.Contains(view, "z\x1b[31m") {
			t.Fatal("directory name leaked escape sequence")
		}
		lines := strings.Split(view, "\n")
		if len(lines) > size[1] {
			t.Fatal("view exceeds terminal height")
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("view exceeds terminal width: %q", line)
			}
		}
	}
	root := filepath.VolumeName(dir) + string(filepath.Separator)
	result := browseCmd(root)().(browserMsg)
	if result.err != nil {
		t.Fatal(result.err)
	}
	for _, folder := range result.folders {
		if folder.name == ".." {
			t.Fatal("root must not have parent entry")
		}
	}
}
