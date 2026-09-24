package cmd

import (
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
)

type folderEntry struct{ name, path string }
type browserMsg struct {
	directory string
	folders   []folderEntry
	err       error
}

func browseCmd(directory string) tea.Cmd {
	return func() tea.Msg {
		directory, err := filepath.Abs(directory)
		if err != nil {
			return browserMsg{err: err}
		}
		result := browserMsg{directory: directory}
		entries, err := os.ReadDir(directory)
		if err != nil {
			result.err = err
			return result
		}
		if parent := filepath.Dir(directory); parent != directory {
			result.folders = append(result.folders, folderEntry{"..", parent})
		}
		for _, entry := range entries {
			// Only real directories: do not follow directory symlinks implicitly.
			if entry.IsDir() {
				result.folders = append(result.folders, folderEntry{entry.Name(), filepath.Join(directory, entry.Name())})
			}
		}
		return result
	}
}

func (m tuiModel) updateBrowser(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		m.browserCursor = max(0, m.browserCursor-1)
	case "down", "j":
		m.browserCursor = min(max(0, len(m.folders)-1), m.browserCursor+1)
	case "pgup":
		m.browserCursor = max(0, m.browserCursor-m.pageSize())
	case "pgdown":
		m.browserCursor = min(max(0, len(m.folders)-1), m.browserCursor+m.pageSize())
	case "home":
		m.browserCursor = 0
	case "end":
		m.browserCursor = max(0, len(m.folders)-1)
	case "enter", "right", "l":
		if len(m.folders) > 0 {
			m.loading = true
			return m, browseCmd(m.folders[m.browserCursor].path)
		}
	case "left", "backspace", "h":
		m.loading = true
		return m, browseCmd(filepath.Dir(m.browserDir))
	case "~":
		home, err := os.UserHomeDir()
		if err != nil {
			m.notice = "Cannot find home folder: " + err.Error()
			return m, nil
		}
		m.loading = true
		return m, browseCmd(home)
	case "r":
		m.loading = true
		return m, browseCmd(m.browserDir)
	case "s":
		if m.browserReady {
			m.browsing, m.loading, m.deleting = false, true, false
			m.plan = filePlan{directory: m.browserDir}
			m.selected, m.cursor = nil, 0
			return m, scanCmd(m.browserDir)
		}
	case "esc":
		return m, tea.Quit
	}
	return m, nil
}
