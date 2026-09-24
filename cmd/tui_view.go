package cmd

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	reset     = "\x1b[0m"
	cyan      = "\x1b[1;36m"
	green     = "\x1b[1;32m"
	yellow    = "\x1b[1;33m"
	red       = "\x1b[1;31m"
	muted     = "\x1b[90m"
	selection = "\x1b[1;7;36m"
)

func highlight(style, text string) string { return style + text + reset }

// Filesystem text must never be interpreted as terminal escape sequences.
func safeText(text string) string {
	quoted := strconv.QuoteToGraphic(text)
	return quoted[1 : len(quoted)-1]
}

func statusStyle(status fileStatus) string {
	switch status {
	case statusPlanned, "READY":
		return cyan
	case statusMoved, statusDeleted:
		return green
	case statusFailed:
		return red
	default:
		return yellow
	}
}

func (m tuiModel) View() tea.View {
	inner := max(1, m.width-4)
	// The border and padding use four cells; truncate before styling/padding.
	row := func(text string) string {
		text = ansi.Truncate(text, inner, "…")
		return highlight(muted, "│") + " " + text + strings.Repeat(" ", max(0, inner-ansi.StringWidth(text))) + " " + highlight(muted, "│")
	}
	selectedRow := func(text string) string {
		text = ansi.Truncate(text, inner, "…")
		return row(highlight(selection, text+strings.Repeat(" ", max(0, inner-ansi.StringWidth(text)))))
	}
	divider := func(title, left, right string) string {
		title = ansi.Truncate(" "+title+" ", max(1, m.width-4), "…")
		return highlight(muted, left+"─") + highlight(cyan, title) + highlight(muted, strings.Repeat("─", max(0, m.width-3-ansi.StringWidth(title)))+right)
	}
	counts := m.plan.counts()
	title, directory := "gofiles / ORGANIZE", m.plan.directory
	summary := fmt.Sprintf("%s selected  %s ready  %s moved  %s skipped  %s failed",
		highlight(cyan, fmt.Sprint(m.selectedCount())), highlight(cyan, fmt.Sprint(counts.planned)),
		highlight(green, fmt.Sprint(counts.moved)), highlight(yellow, fmt.Sprint(counts.skipped)), highlight(red, fmt.Sprint(counts.failed)))
	listTitle, columnTitle := "FILES", "    STATUS  FILE → CATEGORY"
	cursor, length := m.cursor, len(m.plan.items)
	from, to, info := "—", "—", "—"
	notice := safeText(m.notice)
	noticeStyle := muted
	help1 := highlight(cyan, "↑/↓") + " browse  " + highlight(cyan, "space") + " select  " + highlight(cyan, "a") + " all  " + highlight(cyan, "u") + " duplicates  " + highlight(green, "Enter") + " apply"
	help2 := highlight(yellow, "d") + " delete mode  " + highlight(cyan, "b/Esc") + " folders  " + highlight(cyan, "r") + " refresh  " + highlight(cyan, "q") + " quit"
	if length > 0 {
		item := m.plan.items[m.cursor]
		from = safeText(item.source)
		if item.destination != "" {
			to = safeText(item.destination)
		}
		if item.reason != "" {
			info = highlight(statusStyle(item.status), safeText(item.reason))
		}
		if item.duplicate {
			info = highlight(yellow, "DUP: numbered filename; contents not compared")
			if item.reason != "" {
				info += " · " + safeText(item.reason)
			}
		}
	}
	if m.deleting && !m.browsing {
		title = "gofiles / PERMANENT DELETE"
		summary = fmt.Sprintf("%s selected  %s deletable  %s deleted  %s failed",
			highlight(yellow, fmt.Sprint(m.selectedCount())), highlight(cyan, fmt.Sprint(m.eligibleCount())),
			highlight(green, fmt.Sprint(counts.deleted)), highlight(red, fmt.Sprint(counts.failed)))
		help2 = highlight(cyan, "d") + " organize mode  " + highlight(cyan, "b/Esc") + " folders  " + highlight(cyan, "r") + " refresh  " + highlight(cyan, "q") + " quit"
		to = "No destination — permanent deletion"

	}
	if m.browsing {
		title, directory = "gofiles / CHOOSE FOLDER", m.browserDir
		listTitle, columnTitle = "FOLDERS", "    NAME"
		cursor, length = m.browserCursor, len(m.folders)
		summary = highlight(cyan, "BROWSE") + "  Choose a folder before organizing files"
		from, to, info = safeText(m.browserDir), "—", "Press s to preview files in the current folder"
		if length > 0 {
			to = safeText(m.folders[cursor].path)
		}
		help1 = highlight(cyan, "↑/↓") + " browse  " + highlight(cyan, "Enter/→") + " open  " + highlight(cyan, "←/Bksp") + " parent"
		help2 = highlight(green, "s") + " use folder  " + highlight(cyan, "~") + " home  " + highlight(cyan, "r") + " refresh  " + highlight(cyan, "q") + " quit"
	}
	if m.loading {
		notice = "Loading…"
		noticeStyle = cyan
	}
	if m.confirming {
		notice = fmt.Sprintf("Move %d selected files?", m.selectedCount())
		noticeStyle = yellow
		help1 = highlight(green, "y") + " confirm move  " + highlight(yellow, "n/Esc") + " cancel"
		help2 = "Only selected files will move. Existing files are never overwritten."
		if m.deleting {
			notice = fmt.Sprintf("Permanently delete %d files?", m.selectedCount())
			noticeStyle = red
			help1 = highlight(yellow, "y") + " confirm delete  " + highlight(cyan, "n/Esc") + " cancel"
			help2 = "Cannot be undone or restored by this tool."
		}
	}
	if m.applying {
		notice = fmt.Sprintf("Moving files… %d/%d completed", m.completed, m.total)
		noticeStyle = cyan
		help1, help2 = highlight(yellow, "Esc")+" stop after current file", "Completed operations are kept."
		if m.deleting {
			notice = fmt.Sprintf("Permanently deleting… %d/%d completed", m.completed, m.total)
		}
		if m.stopping {
			notice = "Stopping after the current file…"
		}
	}
	if strings.HasPrefix(m.notice, "Cannot ") || strings.HasPrefix(m.notice, "Scan failed:") {
		noticeStyle = red
	}
	duplicateCount := 0
	if !m.browsing {
		for _, item := range m.plan.items {
			if item.duplicate && item.status != statusMoved && item.status != statusDeleted {
				duplicateCount++
			}
		}
		listTitle += fmt.Sprintf(" · %d DUP", duplicateCount)
	}
	lines := []string{
		divider(title, "┌", "┐"),
		row(highlight(cyan, "Path  ") + safeText(directory)),
		row(summary),
		divider(fmt.Sprintf("%s · %d items", listTitle, length), "├", "┤"),
		row(highlight(muted, columnTitle)),
	}
	start := max(0, cursor-m.pageSize()+1)
	end := min(length, start+m.pageSize())
	for i := start; i < end; i++ {
		var plain, styled string
		if m.browsing {
			folder := m.folders[i]
			plain = " >  " + safeText(folder.name) + "/"
			styled = "    " + highlight(cyan, safeText(folder.name)+"/")
			if folder.name == ".." {
				plain, styled = " >  ../  (parent folder)", "    "+highlight(muted, "../  (parent folder)")
			}
		} else {
			item := m.plan.items[i]
			check, target := "[ ]", "—"
			if m.selected[i] {
				check = "[x]"
			}
			if item.destination != "" {
				target = filepath.Base(filepath.Dir(item.destination))
			}
			displayStatus := item.status
			if m.deleting && canDelete(item) {
				if item.status != statusFailed {
					displayStatus = "READY"
				}
				target = "PERMANENT DELETE"
			}
			tag := ""
			if item.duplicate {
				tag = "DUP "
			}
			plain = fmt.Sprintf("%s %-6s %s%s → %s", check, displayStatus, tag, safeText(filepath.Base(item.source)), target)
			styled = highlight(cyan, check) + " " + highlight(statusStyle(displayStatus), fmt.Sprintf("%-6s", displayStatus)) + " " + highlight(yellow, tag) + safeText(filepath.Base(item.source)) + " → " + highlight(cyan, target)

		}
		if i == cursor {
			lines = append(lines, selectedRow(plain))
		} else {
			lines = append(lines, row(styled))
		}
	}
	if length == 0 {
		lines = append(lines, row(highlight(muted, "(no entries)")))
	}
	for len(lines) < 5+m.pageSize() {
		lines = append(lines, row(""))
	}
	detailTitle := "DETAILS"
	labels := []string{"From  ", "To    ", "Info  "}
	if m.browsing {
		detailTitle = "FOLDER DETAILS"
		labels = []string{"Use   ", "Open  ", "Info  "}
	}
	lines = append(lines,
		divider(detailTitle, "├", "┤"),
		row(highlight(cyan, labels[0])+from),
		row(highlight(cyan, labels[1])+to),
		row(highlight(cyan, labels[2])+info),
		divider("ACTIONS", "├", "┤"),
		row(highlight(noticeStyle, notice)), row(help1), row(help2),
		highlight(muted, "└"+strings.Repeat("─", max(0, m.width-2))+"┘"),
	)
	if m.height < 15 || m.width < 35 {
		lines = []string{highlight(cyan, "gofiles"), "Enlarge terminal to at least 35×15.", notice, "q quit · Esc cancel/stop"}
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], m.width, "…")
	}
	if len(lines) > m.height {
		lines = lines[:m.height]
	}
	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	return view
}
