package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestDeleteModeConfirmationAndDuplicates(t *testing.T) {
	dir := t.TempDir()
	put(t, filepath.Join(dir, "normal.pdf"), "keep")
	put(t, filepath.Join(dir, "report (1).pdf"), "first")
	put(t, filepath.Join(dir, "report (2).pdf"), "second")
	m := loadedTUI(t, dir)
	m, cmd := press(m, 'd')
	if !m.deleting || m.selectedCount() != 0 || cmd != nil {
		t.Fatal("entering delete mode must clear inherited selections without changing files")
	}
	m, cmd = press(m, tea.KeyEnter)
	if m.confirming || cmd != nil {
		t.Fatal("empty selection must not start deletion")
	}
	m, _ = press(m, 'u')
	if m.selectedCount() != 2 {
		t.Fatal("u should select only numbered duplicates")
	}
	m, cmd = press(m, tea.KeyEnter)
	if !m.confirming || cmd != nil {
		t.Fatal("enter only opens confirmation")
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "Permanently delete 2 files?") || !strings.Contains(view, "Cannot be undone") {
		t.Fatal(view)
	}
	m, cmd = press(m, 'n')
	if m.confirming || cmd != nil {
		t.Fatal("n should cancel")
	}
	contentIs(t, filepath.Join(dir, "report (1).pdf"), "first")
	if _, err := os.Lstat(filepath.Join(dir, ".gooooo-trash")); !os.IsNotExist(err) {
		t.Fatal("cancel created trash")
	}
	m, _ = press(m, tea.KeyEnter)
	m, cmd = press(m, 'y')
	m = drainMoves(t, m, cmd)
	if m.plan.counts().deleted != 2 || m.exitErr != nil {
		t.Fatalf("unexpected results: %+v, %v", m.plan.counts(), m.exitErr)
	}
	contentIs(t, filepath.Join(dir, "normal.pdf"), "keep")
	for _, item := range m.plan.items {
		if !item.duplicate {
			continue
		}
		if _, err := os.Lstat(item.source); !os.IsNotExist(err) {
			t.Fatal("deleted source still exists")
		}
		if item.destination != "" {
			t.Fatal("permanent deletion must not create a destination")
		}

	}
	if _, err := os.Lstat(filepath.Join(dir, ".gooooo-trash")); !os.IsNotExist(err) {
		t.Fatal("permanent deletion created trash")
	}
	// Refresh in delete mode must not select the remaining normal file.
	m, cmd = press(m, 'r')
	m, _ = updateTUI(m, cmd())
	if m.selectedCount() != 0 {
		t.Fatal("refresh must not select files for deletion")
	}
	m, _ = press(m, 'u')
	if m.selectedCount() != 0 {
		t.Fatal("deleted files must not become duplicate candidates")
	}
}

func TestDeleteOrdinaryFilePreservesExistingTrash(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "ordinary.txt")
	put(t, source, "delete me")
	oldTrash := filepath.Join(dir, ".gooooo-trash")
	if err := os.Mkdir(oldTrash, 0700); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(oldTrash, "old.txt"), "keep old trash")
	m := loadedTUI(t, dir)
	m, _ = press(m, 'd')
	for i, item := range m.plan.items {
		if item.source == source {
			m.cursor = i
		}
	}
	m, _ = press(m, tea.KeySpace)
	if m.selectedCount() != 1 {
		t.Fatal("normal files must be manually deletable")
	}
	m, _ = press(m, tea.KeyEnter)
	m, cmd := press(m, 'y')
	m = drainMoves(t, m, cmd)
	if m.plan.items[m.cursor].status != statusDeleted {
		t.Fatal("expected deletion")
	}
	if _, err := os.Lstat(source); !os.IsNotExist(err) {
		t.Fatal("source still exists")
	}
	contentIs(t, filepath.Join(oldTrash, "old.txt"), "keep old trash")
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatal("deletion created unexpected files")
	}
}

func TestDeleteRevalidatesAndContinues(t *testing.T) {
	dir := t.TempDir()
	put(t, filepath.Join(dir, "a (1).txt"), "original")
	put(t, filepath.Join(dir, "b (2).txt"), "second")
	m := loadedTUI(t, dir)
	m, _ = press(m, 'd')
	m, _ = press(m, 'u')
	m, _ = press(m, tea.KeyEnter)
	put(t, filepath.Join(dir, "a (1).txt"), "changed since preview")
	put(t, filepath.Join(dir, "new (3).txt"), "new arrival")
	m, cmd := press(m, 'y')
	m = drainMoves(t, m, cmd)
	if m.plan.counts().failed != 1 || m.plan.counts().deleted != 1 || m.exitErr == nil {
		t.Fatalf("unexpected results: %+v", m.plan.counts())
	}
	contentIs(t, filepath.Join(dir, "a (1).txt"), "changed since preview")
	contentIs(t, filepath.Join(dir, "new (3).txt"), "new arrival")
}

func TestDeleteStopAndModeSwitch(t *testing.T) {
	dir := t.TempDir()
	put(t, filepath.Join(dir, "a.txt"), "a")
	put(t, filepath.Join(dir, "b.txt"), "b")
	m := loadedTUI(t, dir)
	m, _ = press(m, 'd')
	m, _ = press(m, 'a')
	if m.selectedCount() != 2 {
		t.Fatal("select all in delete mode")
	}
	m, _ = press(m, tea.KeyEnter)
	m, cmd := press(m, 'y')
	m, _ = press(m, tea.KeyEscape)
	m, next := updateTUI(m, cmd())
	if m.applying || next != nil || m.plan.counts().deleted != 1 {
		t.Fatal("stop should complete only current deletion")
	}
	contentIs(t, filepath.Join(dir, "b.txt"), "b")
	m, _ = press(m, 'd')
	if m.deleting || m.selectedCount() != 0 {
		t.Fatal("switching mode must clear selection")
	}
}

func TestDeleteRejectsChangedAndSpecialSources(t *testing.T) {
	for _, change := range []string{"deleted", "symlink", "directory", "replacement"} {
		t.Run(change, func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join(dir, "file.txt")
			put(t, source, "original")
			plan, err := scanDirectory(dir)
			if err != nil {
				t.Fatal(err)
			}
			// Keep the original inode alive to make identity replacement unambiguous.
			if err := os.Rename(source, filepath.Join(dir, ".original")); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "symlink":
				if err := os.Symlink(".original", source); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(source, 0755); err != nil {
					t.Fatal(err)
				}
			case "replacement":
				put(t, source, "replacement")
			}
			result := deleteItem(plan.items[0])
			if result.status != statusFailed {
				t.Fatal("changed source must be rejected")
			}
			contentIs(t, filepath.Join(dir, ".original"), "original")
			if _, err := os.Lstat(filepath.Join(dir, ".gooooo-trash")); !os.IsNotExist(err) {
				t.Fatal("invalid source created trash")
			}
		})
	}
}

func TestDeleteEligibility(t *testing.T) {
	dir := t.TempDir()
	put(t, filepath.Join(dir, ".hidden (1)"), "hidden")
	if err := os.Mkdir(filepath.Join(dir, "folder (1)"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", filepath.Join(dir, "link (1)")); err != nil {
		t.Fatal(err)
	}
	plan, err := scanDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range plan.items {
		if canDelete(item) {
			t.Fatalf("special file eligible for deletion: %s", item.source)
		}
	}
}

func TestDeleteCanHandleOrganizeConflict(t *testing.T) {
	dir := t.TempDir()
	put(t, filepath.Join(dir, "a (1).pdf"), "duplicate")
	if err := os.Mkdir(filepath.Join(dir, "Documents"), 0755); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(dir, "Documents", "a (1).pdf"), "existing")
	m := loadedTUI(t, dir)
	m, _ = press(m, 'd')
	m, _ = press(m, 'u')
	if m.selectedCount() != 1 {
		t.Fatal("organize conflict should not prevent deletion of source")
	}
	m, _ = press(m, tea.KeyEnter)
	m, cmd := press(m, 'y')
	m = drainMoves(t, m, cmd)
	if m.plan.counts().deleted != 1 {
		t.Fatal("expected deletion")
	}
	contentIs(t, filepath.Join(dir, "Documents", "a (1).pdf"), "existing")
}
