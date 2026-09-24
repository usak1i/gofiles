package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
)

func newTUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui [directory]",
		Short: "Interactively preview, select, and organize files (starts with a directory browser)",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runTUI,
	}
}

func runTUI(cmd *cobra.Command, args []string) error {
	input, inputOK := cmd.InOrStdin().(*os.File)
	output, outputOK := cmd.OutOrStdout().(*os.File)
	if !inputOK || !outputOK || !term.IsTerminal(input.Fd()) || !term.IsTerminal(output.Fd()) {
		return fmt.Errorf("TUI requires an interactive terminal; use 'gofiles organize <directory>' for a text preview")
	}
	directory := "."
	if len(args) > 0 {
		directory = args[0]
	}
	directory, err := filepath.Abs(directory)
	if err != nil {
		return err
	}
	model := initialTUIModel(directory, len(args) > 0)
	final, err := tea.NewProgram(model, tea.WithInput(input), tea.WithOutput(output)).Run()
	if err != nil {
		return err
	}
	return final.(tuiModel).exitErr
}

type scanMsg struct {
	plan filePlan
	err  error
}
type movedMsg struct {
	index int
	item  fileItem
}

type tuiModel struct {
	browsing      bool
	browserDir    string
	folders       []folderEntry
	browserCursor int
	browserReady  bool
	deleting      bool

	plan                                    filePlan
	selected                                []bool
	cursor, width, height                   int
	loading, confirming, applying, stopping bool
	queue                                   []int
	completed, total                        int
	notice                                  string
	exitErr                                 error
}

func newTUIModel(directory string) tuiModel {
	return tuiModel{plan: filePlan{directory: directory}, width: 80, height: 24, loading: true}
}

func scanCmd(directory string) tea.Cmd {
	return func() tea.Msg {
		plan, err := scanDirectory(directory)
		return scanMsg{plan, err}
	}
}

func moveCmd(index int, item fileItem) tea.Cmd {
	return func() tea.Msg { return movedMsg{index, applyItem(item)} }
}

func initialTUIModel(directory string, explicitDirectory bool) tuiModel {
	m := newTUIModel(directory)
	if !explicitDirectory {
		m.browsing, m.browserDir = true, directory
	}
	return m
}

func (m tuiModel) Init() tea.Cmd {
	if m.browsing {
		return browseCmd(m.browserDir)
	}
	return scanCmd(m.plan.directory)
}

func (m tuiModel) selectedCount() int {
	count := 0
	for i, selected := range m.selected {
		if selected && m.eligible(m.plan.items[i]) {
			count++
		}
	}
	return count
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = max(1, msg.Width), max(1, msg.Height)
	case browserMsg:
		m.loading = false
		if msg.err != nil {
			m.notice = "Cannot open folder: " + msg.err.Error()
			return m, nil
		}
		m.browserDir, m.folders, m.browserCursor = msg.directory, msg.folders, 0
		m.browserReady = true
		m.notice = "Browse folders, then press s to organize the current folder."
	case scanMsg:
		m.loading = false
		if msg.err != nil {
			m.plan.items, m.selected = nil, nil
			m.exitErr, m.notice = msg.err, "Scan failed: "+msg.err.Error()
			return m, nil
		}
		m.plan, m.cursor = msg.plan, 0
		m.selected = make([]bool, len(m.plan.items))
		for i, item := range m.plan.items {
			m.selected[i] = !m.deleting && item.status == statusPlanned
		}
		m.notice = "Preview only. Enter reviews moves; d switches to delete mode."
		if m.deleting {
			m.notice = "Delete mode: select files, or press u for duplicates. Enter reviews deletion."
		}
		if len(m.plan.items) == 0 {
			m.notice = "This directory is empty. Press r to refresh."
		}
	case movedMsg:
		m.plan.items[msg.index] = msg.item
		m.selected[msg.index] = false
		m.completed++
		if msg.item.status == statusFailed {
			m.exitErr = fmt.Errorf("one or more file operations failed")
		}
		if m.stopping {
			m.applying, m.stopping, m.queue = false, false, nil
			m.notice = fmt.Sprintf("Stopped after %d of %d files. Completed operations are kept.", m.completed, m.total)
			return m, nil
		}
		if len(m.queue) > 0 {
			index := m.queue[0]
			m.queue = m.queue[1:]
			return m, m.processCmd(index)
		}
		m.applying = false
		m.notice = "Finished. Review results below; r refreshes, q exits."
	case tea.KeyPressMsg:
		key := msg.String()
		if m.applying {
			if key == "q" || key == "ctrl+c" || key == "esc" {
				m.stopping = true
			}
			return m, nil
		}
		if key == "q" || key == "ctrl+c" {
			return m, tea.Quit
		}
		if m.loading {
			return m, nil
		}
		if m.browsing {
			return m.updateBrowser(key)
		}
		if m.confirming {
			switch key {
			case "esc", "n":
				m.confirming = false
			case "y":
				m.confirming, m.applying = false, true
				m.queue = nil
				for i, selected := range m.selected {
					if selected && m.eligible(m.plan.items[i]) {
						m.queue = append(m.queue, i)
					}
				}
				m.completed, m.total = 0, len(m.queue)
				if !m.deleting {
					if err := m.plan.counts().err(); err != nil {
						m.exitErr = err
					}
				}
				if len(m.queue) == 0 {
					m.applying = false
					return m, nil
				}
				index := m.queue[0]
				m.queue = m.queue[1:]
				return m, m.processCmd(index)
			}
			return m, nil
		}
		switch key {
		case "up", "k":
			m.cursor = max(0, m.cursor-1)
		case "down", "j":
			m.cursor = min(max(0, len(m.plan.items)-1), m.cursor+1)
		case "pgup":
			m.cursor = max(0, m.cursor-m.pageSize())
		case "pgdown":
			m.cursor = min(max(0, len(m.plan.items)-1), m.cursor+m.pageSize())
		case "home":
			m.cursor = 0
		case "end":
			m.cursor = max(0, len(m.plan.items)-1)
		case "space":
			if len(m.selected) > 0 && m.eligible(m.plan.items[m.cursor]) {
				m.selected[m.cursor] = !m.selected[m.cursor]
			}
		case "a":
			selectAll := m.selectedCount() != m.eligibleCount()
			for i, item := range m.plan.items {
				m.selected[i] = selectAll && m.eligible(item)
			}
		case "d":
			m.deleting = !m.deleting
			for i := range m.selected {
				m.selected[i] = false
			}
			m.notice = "Organize mode: select files to move."
			if m.deleting {
				m.notice = "Delete mode: nothing selected. u selects duplicates; Enter reviews deletion."
			}
		case "u":
			for i, item := range m.plan.items {
				m.selected[i] = item.duplicate && m.eligible(item)
			}
			m.notice = fmt.Sprintf("Selected %d suspected duplicates by filename only.", m.selectedCount())
		case "enter":
			if m.selectedCount() > 0 {
				m.confirming = true
			} else {
				m.notice = "No eligible files selected."
			}
		case "r":
			m.loading, m.notice = true, ""
			return m, scanCmd(m.plan.directory)
		case "b", "esc":
			m.browsing, m.loading = true, true
			m.browserDir = m.plan.directory
			m.browserReady = false
			m.folders = nil
			m.browserCursor = 0
			return m, browseCmd(m.browserDir)
		}
	}
	return m, nil
}

func (m tuiModel) pageSize() int { return max(1, m.height-14) }

func (m tuiModel) eligible(item fileItem) bool {
	if m.deleting {
		return canDelete(item)
	}
	return item.status == statusPlanned
}

func (m tuiModel) eligibleCount() int {
	n := 0
	for _, item := range m.plan.items {
		if m.eligible(item) {
			n++
		}
	}
	return n
}

func (m tuiModel) processCmd(index int) tea.Cmd {
	item := m.plan.items[index]
	if m.deleting {
		return func() tea.Msg { return movedMsg{index, deleteItem(item)} }
	}
	return moveCmd(index, item)
}
