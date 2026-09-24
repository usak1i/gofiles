package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func updateTUI(m tuiModel, msg tea.Msg) (tuiModel, tea.Cmd) {
	updated, cmd := m.Update(msg)
	return updated.(tuiModel), cmd
}

func press(m tuiModel, code rune) (tuiModel, tea.Cmd) {
	return updateTUI(m, tea.KeyPressMsg{Code: code})
}

func loadedTUI(t *testing.T, dir string) tuiModel {
	t.Helper()
	m := newTUIModel(dir)
	m, _ = updateTUI(m, m.Init()())
	if m.exitErr != nil {
		t.Fatal(m.exitErr)
	}
	return m
}

func drainMoves(t *testing.T, m tuiModel, cmd tea.Cmd) tuiModel {
	t.Helper()
	for count := 0; cmd != nil; count++ {
		if count > 100 {
			t.Fatal("move loop did not terminate")
		}
		m, cmd = updateTUI(m, cmd())
	}
	return m
}

func TestTUISelectionAndConfirmation(t *testing.T) {
	dir := t.TempDir()
	put(t, filepath.Join(dir, "a.pdf"), "a")
	put(t, filepath.Join(dir, "b.png"), "b")
	m := loadedTUI(t, dir)
	if m.selectedCount() != 2 {
		t.Fatal("eligible files should initially be selected")
	}
	m, _ = press(m, tea.KeySpace)
	if m.selectedCount() != 1 {
		t.Fatal("space should deselect row")
	}
	m, command := press(m, tea.KeyEnter)
	if !m.confirming || command != nil {
		t.Fatal("enter must only open confirmation")
	}
	// Pressing Enter again cannot accidentally confirm an operation.
	m, command = press(m, tea.KeyEnter)
	if !m.confirming || command != nil {
		t.Fatal("confirmation requires explicit y")
	}
	m, _ = press(m, tea.KeyEscape)
	if m.confirming {
		t.Fatal("escape should cancel")
	}
	contentIs(t, filepath.Join(dir, "a.pdf"), "a")
	contentIs(t, filepath.Join(dir, "b.png"), "b")
	// Files created after preview must not join the confirmed operation.
	put(t, filepath.Join(dir, "new.txt"), "new")
	m, _ = press(m, tea.KeyEnter)
	m, command = press(m, 'y')
	if !m.applying || command == nil {
		t.Fatal("y should schedule the selected move")
	}
	m = drainMoves(t, m, command)
	if m.applying || m.plan.counts().moved != 1 {
		t.Fatalf("unexpected results: %+v", m.plan.counts())
	}
	contentIs(t, filepath.Join(dir, "Images", "b.png"), "b")
	contentIs(t, filepath.Join(dir, "a.pdf"), "a")
	contentIs(t, filepath.Join(dir, "new.txt"), "new")
	m, command = press(m, 'r')
	if !m.loading || command == nil {
		t.Fatal("refresh must scan again")
	}
	m, _ = updateTUI(m, command())
	if m.selectedCount() != 2 {
		t.Fatalf("expected two remaining files, got %d", m.selectedCount())
	}
}

func TestTUIStopWaitsForCurrentFile(t *testing.T) {
	dir := t.TempDir()
	put(t, filepath.Join(dir, "a.pdf"), "a")
	put(t, filepath.Join(dir, "b.pdf"), "b")
	m := loadedTUI(t, dir)
	m, _ = press(m, tea.KeyEnter)
	m, move := press(m, 'y')
	m, quit := press(m, 'q')
	if !m.stopping || quit != nil {
		t.Fatal("quit should wait for the pending move")
	}
	m, next := updateTUI(m, move())
	if m.applying || next != nil {
		t.Fatal("stop should prevent the next move")
	}
	contentIs(t, filepath.Join(dir, "Documents", "a.pdf"), "a")
	contentIs(t, filepath.Join(dir, "b.pdf"), "b")
	_, quit = press(m, 'q')
	if quit == nil {
		t.Fatal("q should exit when no move is pending")
	}
}

func TestTUIFailuresContinue(t *testing.T) {
	dir := t.TempDir()
	put(t, filepath.Join(dir, "a.pdf"), "original")
	put(t, filepath.Join(dir, "b.pdf"), "b")
	m := loadedTUI(t, dir)
	put(t, filepath.Join(dir, "a.pdf"), "changed since preview")
	m, _ = press(m, tea.KeyEnter)
	m, command := press(m, 'y')
	m = drainMoves(t, m, command)
	if m.exitErr == nil || m.plan.counts().failed != 1 || m.plan.counts().moved != 1 {
		t.Fatalf("unexpected results: %+v", m.plan.counts())
	}
	contentIs(t, filepath.Join(dir, "a.pdf"), "changed since preview")
	contentIs(t, filepath.Join(dir, "Documents", "b.pdf"), "b")
}

func TestTUIEmptySkippedAndNavigation(t *testing.T) {
	dir := t.TempDir()
	m := loadedTUI(t, dir)
	for _, key := range []rune{tea.KeyUp, tea.KeyDown, tea.KeySpace, 'a', tea.KeyEnter} {
		m, _ = press(m, key)
	}
	if m.confirming || m.selectedCount() != 0 {
		t.Fatal("empty list should not permit apply")
	}
	put(t, filepath.Join(dir, ".hidden"), "hidden")
	for i := 0; i < 40; i++ {
		put(t, filepath.Join(dir, fmt.Sprintf("%02d.txt", i)), "file")
	}
	m = loadedTUI(t, dir)
	m, _ = press(m, tea.KeySpace)
	if m.selectedCount() != 40 {
		t.Fatal("hidden entry must not become selectable")
	}
	m, _ = press(m, 'a')
	if m.selectedCount() != 0 {
		t.Fatal("a should deselect all")
	}
	m, _ = press(m, 'a')
	if m.selectedCount() != 40 {
		t.Fatal("a should select eligible entries only")
	}
	for i := 0; i < 100; i++ {
		m, _ = press(m, tea.KeyDown)
	}
	if m.cursor != 40 {
		t.Fatalf("cursor out of range: %d", m.cursor)
	}
	for _, size := range [][2]int{{80, 24}, {35, 15}, {20, 5}, {1, 1}} {
		m, _ = updateTUI(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view := m.View()
		if !view.AltScreen {
			t.Fatal("expected alternate screen")
		}
		lines := strings.Split(view.Content, "\n")
		if len(lines) > size[1] {
			t.Fatalf("too many lines for %v", size)
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("line too wide for %v: %q", size, line)
			}
		}
	}
}

func TestTUIEscapesFilenames(t *testing.T) {
	dir := t.TempDir()
	put(t, filepath.Join(dir, "danger\x1b[31m\n.txt"), "data")
	m := loadedTUI(t, dir)
	m.width = 200
	view := m.View().Content
	if strings.Contains(view, "danger\x1b") || !strings.Contains(view, `danger\x1b[31m\n.txt`) {
		t.Fatalf("unsafe filename rendering: %q", view)
	}
}

func TestTUINonTerminalAndScanError(t *testing.T) {
	for _, args := range [][]string{{}, {"tui"}, {"tui", "."}} {
		_, err := runCLI(args...)
		if err == nil || !strings.Contains(err.Error(), "interactive terminal") {
			t.Fatalf("expected terminal error for %v: %v", args, err)
		}
	}
	m := newTUIModel(filepath.Join(t.TempDir(), "missing"))
	m, _ = updateTUI(m, m.Init()())
	if m.loading || m.exitErr == nil || !strings.Contains(m.notice, "Scan failed") {
		t.Fatal("expected visible scan error")
	}
	_, quit := press(m, 'q')
	if quit == nil {
		t.Fatal("scan failure must allow exit")
	}
}

func TestApplyRechecksPreview(t *testing.T) {
	for _, change := range []string{"deleted", "symlink", "destination", "category symlink"} {
		t.Run(change, func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join(dir, "a.pdf")
			put(t, source, "a")
			plan, err := scanDirectory(dir)
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "deleted":
				if err := os.Remove(source); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Remove(source); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("missing", source); err != nil {
					t.Fatal(err)
				}
			case "destination":
				if err := os.Mkdir(filepath.Join(dir, "Documents"), 0755); err != nil {
					t.Fatal(err)
				}
				put(t, filepath.Join(dir, "Documents", "a.pdf"), "existing")
			case "category symlink":
				if err := os.Symlink(t.TempDir(), filepath.Join(dir, "Documents")); err != nil {
					t.Fatal(err)
				}
			}
			result := applyItem(plan.items[0])
			if result.status != statusFailed && result.status != statusSkipped {
				t.Fatalf("unexpected move: %+v", result)
			}
			if change == "destination" {
				contentIs(t, source, "a")
				contentIs(t, filepath.Join(dir, "Documents", "a.pdf"), "existing")
			}
		})
	}
}
