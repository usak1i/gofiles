package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func newOrganizeCmd() *cobra.Command {
	var apply bool
	cmd := &cobra.Command{
		Use:   "organize <directory>",
		Short: "Preview file classification; use --apply to move files",
		Args:  cobra.ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return organize(args[0], apply, cmd.OutOrStdout()) },
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "Move files into category directories")
	return cmd
}

func category(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".pdf", ".txt", ".md", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".csv":
		return "Documents"
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".svg", ".heic":
		return "Images"
	case ".zip", ".rar", ".7z", ".tar", ".gz", ".bz2", ".xz":
		return "Archives"
	case ".mp3", ".wav", ".flac", ".m4a", ".aac", ".ogg":
		return "Audio"
	case ".mp4", ".mov", ".mkv", ".avi", ".webm":
		return "Video"
	default:
		return "Others"
	}
}

func organize(directory string, apply bool, out io.Writer) error {
	plan, err := scanDirectory(directory)
	if err != nil {
		return err
	}
	for i, item := range plan.items {
		if apply && item.status == statusPlanned {
			item = applyItem(item)
			plan.items[i] = item
		}
		fmt.Fprintln(out, item.line())
	}
	counts := plan.counts()
	if apply {
		fmt.Fprintf(out, "Moved: %d, skipped: %d, failed: %d\n", counts.moved, counts.skipped, counts.failed)
	} else {
		fmt.Fprintf(out, "Preview: %d planned, %d skipped, %d failed. Use --apply to move files.\n", counts.planned, counts.skipped, counts.failed)
	}
	return counts.err()
}

func checkCategoryDir(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	// Lstat rejects symlinks as category directories, including dangling links.
	if !info.IsDir() {
		return fmt.Errorf("category path %q is not a directory", path)
	}
	return nil
}

func moveWithoutOverwrite(source, destination string) error {
	// Linking atomically refuses existing destinations, unlike os.Rename.
	// Remove the original only after the destination has been created.
	if err := os.Link(source, destination); err != nil {
		return err
	}
	if err := os.Remove(source); err != nil {
		return fmt.Errorf("destination created but original could not be removed (both paths retained): %w", err)
	}
	return nil
}
