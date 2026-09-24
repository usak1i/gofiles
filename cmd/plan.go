package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type fileStatus string

const (
	statusPlanned fileStatus = "PLAN"
	statusSkipped fileStatus = "SKIP"
	statusFailed  fileStatus = "FAIL"
	statusMoved   fileStatus = "MOVE"
	statusDeleted fileStatus = "DELETE"
)

type fileItem struct {
	source, destination string
	status              fileStatus
	reason              string
	snapshot            os.FileInfo
	duplicate           bool
}

var duplicateNamePattern = regexp.MustCompile(`\([1-9][0-9]*\)`)

func isDuplicateName(name string) bool { return duplicateNamePattern.MatchString(name) }

type filePlan struct {
	directory string
	items     []fileItem
}

type fileCounts struct{ planned, moved, skipped, failed, deleted int }

func (p filePlan) counts() fileCounts {
	var c fileCounts
	for _, item := range p.items {
		switch item.status {
		case statusPlanned:
			c.planned++
		case statusMoved:
			c.moved++
		case statusDeleted:
			c.deleted++
		case statusSkipped:
			c.skipped++
		case statusFailed:
			c.failed++
		}
	}
	return c
}

func (c fileCounts) err() error {
	if c.failed > 0 {
		return fmt.Errorf("could not organize %d file(s)", c.failed)
	}
	return nil
}

func (item fileItem) line() string {
	line := fmt.Sprintf("%s %q", item.status, item.source)
	if item.destination != "" {
		line += fmt.Sprintf(" -> %q", item.destination)
	}
	if item.reason != "" {
		line += fmt.Sprintf(" (%s)", item.reason)
	}
	if item.duplicate {
		line += " [DUP: numbered filename]"
	}
	return line
}

func scanDirectory(directory string) (filePlan, error) {
	directory, err := filepath.Abs(directory)
	if err != nil {
		return filePlan{}, err
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return filePlan{}, fmt.Errorf("read directory: %w", err)
	}
	plan := filePlan{directory: directory}
	// All decisions precede any moves, keeping previews and apply consistent.
	for _, entry := range entries {
		item := fileItem{source: filepath.Join(directory, entry.Name()), status: statusPlanned}
		switch {
		case strings.HasPrefix(entry.Name(), "."):
			item.status, item.reason = statusSkipped, "hidden"
		default:
			info, err := entry.Info()
			switch {
			case err != nil:
				item.status, item.reason = statusFailed, err.Error()
			case !info.Mode().IsRegular():
				item.status, item.reason = statusSkipped, "not a regular file"
			default:
				item.snapshot = info
				item.duplicate = isDuplicateName(entry.Name())
				item.destination = filepath.Join(directory, category(entry.Name()), entry.Name())
				item = checkDestination(item)
			}
		}
		plan.items = append(plan.items, item)
	}
	return plan, nil
}

func checkDestination(item fileItem) fileItem {
	if err := checkCategoryDir(filepath.Dir(item.destination)); err != nil {
		item.status, item.reason = statusFailed, err.Error()
	} else if _, err := os.Lstat(item.destination); err == nil {
		item.status, item.reason = statusSkipped, "destination exists"
	} else if !os.IsNotExist(err) {
		item.status, item.reason = statusFailed, err.Error()
	}
	return item
}

func applyItem(item fileItem) fileItem {
	if item.status != statusPlanned {
		return item
	}
	if err := verifySource(item); err != nil {
		item.status, item.reason = statusFailed, err.Error()
		return item
	}
	item = checkDestination(item)
	if item.status != statusPlanned {
		return item
	}
	destinationDir := filepath.Dir(item.destination)
	if err := os.Mkdir(destinationDir, 0755); err != nil && !os.IsExist(err) {
		item.status, item.reason = statusFailed, err.Error()
		return item
	}
	if err := checkCategoryDir(destinationDir); err != nil {
		item.status, item.reason = statusFailed, err.Error()
		return item
	}
	if err := moveWithoutOverwrite(item.source, item.destination); err != nil {
		if os.IsExist(err) {
			item.status, item.reason = statusSkipped, "destination exists"
		} else {
			item.status, item.reason = statusFailed, err.Error()
		}
		return item
	}
	item.status, item.reason = statusMoved, ""
	return item
}

// A preview can remain open while another process changes the files.
func verifySource(item fileItem) error {
	info, err := os.Lstat(item.source)
	if err != nil {
		return err
	}
	if item.snapshot == nil || !info.Mode().IsRegular() || !os.SameFile(info, item.snapshot) ||
		info.Size() != item.snapshot.Size() || !info.ModTime().Equal(item.snapshot.ModTime()) {
		return fmt.Errorf("source changed since preview; refresh before retrying")
	}
	return nil
}
