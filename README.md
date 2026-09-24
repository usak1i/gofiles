# gofiles

A Go terminal app for organizing a download folder by file extension, with an
interactive TUI and a scriptable CLI.

## Usage

Requires Go 1.25.3 or newer.

```sh
go build -o gooooo .
./gooooo
# Or run from source:
go run .
```

Running without arguments opens a folder browser, starting in the current
directory. Browse to the folder you want and press `s` to preview its files.
You can also use `gooooo tui` to browse, or `gooooo "$HOME/Downloads"` to open a
known directory's preview directly. The TUI requires an interactive terminal on both stdin
and stdout. It uses [Bubble Tea](https://github.com/charmbracelet/bubbletea).

### Interactive mode

The screen has bordered sections for the current path and summary, the list,
details, and actions. The active row uses reverse-video highlighting (also visible
with `NO_COLOR`); ready files are cyan,
completed moves green, skipped files yellow, and failures red. Status labels
remain visible alongside colors.

In the folder browser:

| Key | Action |
| --- | --- |
| Up / Down, j / k | Highlight a folder |
| Enter / Right, l | Open the highlighted folder |
| Left / Backspace, h | Go to the parent folder |
| s | Use the current folder and open its file preview |
| ~ | Go to the home directory |
| r | Refresh the folder list |
| q / Esc / Ctrl+C | Quit |

The browser lists real directories, including hidden folders; directory symlinks
are omitted. The `..` entry opens the parent directory. Empty folders can still be
selected. Failed navigation shows an error and keeps the previous folder open.
Browsing and opening a preview never move files.

In the file preview, the screen lists each file's classification, selection, and
status, with the highlighted file's source, destination, and skip/failure reason below. All eligible
files start selected; opening the app does not move anything.

| Key | Action |
| --- | --- |
| Up / Down, j / k | Browse files |
| Page Up / Page Down, Home / End | Navigate long lists |
| Space | Toggle the highlighted eligible file |
| a | Select or deselect all eligible files |
| u | Replace selection with suspected duplicates only |
| d | Switch between organize and delete mode; clears selection |
| Enter | Review confirmation for selected files |
| y | Confirm the displayed move or deletion |
| n / Esc | Cancel confirmation |
| r | Refresh the preview and reset selection |
| b / Esc | Return to the folder browser to choose another directory |
| q / Ctrl+C | Exit when idle |
| Esc / q / Ctrl+C during an operation | Stop after the current file; keep completed operations |

Moves operate on the selected preview, not a new scan. Files added afterward are
left alone. Sources that disappeared or changed identity, size, or modification
time are reported as failures; new destination conflicts are skipped. Refresh to
review the current folder state. Failures remain visible, other selected files
continue, and a scan or apply failure causes a nonzero exit status when you quit.
Use a terminal at least 35 columns wide and 15 rows high; longer lists scroll and
long paths are truncated to fit.

### Delete files and select suspected duplicates

Files whose names contain `(1)`, `(2)`, or another positive integer in ASCII
parentheses receive a yellow `DUP` marker. For example, `photo (12).jpg` and
`notes(3)edited.txt` match. `(0)`, negative numbers, and nonnumeric parentheses do
not match. This is a filename heuristic: contents are not compared, and an
unnumbered original does not need to exist. The text CLI also displays `DUP`.

To clean up these files:

1. Browse to the folder and press `s` to open its preview.
2. Press `d` to enter **delete mode**. Nothing starts selected in this mode.
3. Press `u` to select only suspected duplicates, or use Space to select files
   individually. Ordinary files can also be deleted. Use `a` to toggle all eligible files.
4. Press Enter to review the deletion, then `y` to confirm, or `n` / Esc to cancel.

Deletion is **permanent**. The confirmation explicitly warns that the operation
cannot be undone or restored by this tool. Files are removed directly; they are
not moved to local trash or the operating system's Trash. Existing
`.gooooo-trash` folders from previous versions are left untouched.

Only non-hidden regular files from the displayed preview can be deleted;
directories, symlinks, and already moved/deleted entries are excluded. Organize
destination conflicts do not prevent deleting the source. Refreshing in delete
mode clears selection; changing folders returns to organize mode. Source files
are rechecked before deletion, and files added after the preview are not included.

### Text mode

The existing commands remain available for scripts and non-interactive terminals:

```sh
./gooooo organize "$HOME/Downloads"
./gooooo organize "$HOME/Downloads" --apply
```

The `organize` command only previews source and destination paths. `--apply` creates
category directories as needed and moves files, reporting moved, skipped, and
failed counts. Individual failures do not stop other files; any failure results
in a nonzero exit status. Existing destinations are skipped without overwriting.

| Directory | Extensions (case insensitive) |
| --- | --- |
| Documents | pdf, txt, md, doc, docx, xls, xlsx, ppt, pptx, csv |
| Images | jpg, jpeg, png, gif, webp, svg, heic |
| Archives | zip, rar, 7z, tar, gz, bz2, xz |
| Audio | mp3, wav, flac, m4a, aac, ogg |
| Video | mp4, mov, mkv, avi, webm |
| Others | All other extensions, including files without an extension |

Only regular files directly inside the supplied directory are organized.
Subdirectories, dotfiles, and symbolic links are skipped. Original filenames
are preserved. Category paths that are files or symbolic links are rejected.
Running the command again leaves already classified files in place.

Moves use a hard link followed by removal of the original to prevent replacing
an existing destination, including one created after the preview/check. The
filesystem must support hard links, and source and destination must be on the
same filesystem. Unsupported moves fail with the source retained. If removing
the original fails, both paths remain and the failure is reported.

The first version does not rename files, compare file contents, automatically
delete duplicates, watch directories, or accept custom classification rules.

## Development

```sh
go test ./...
go vet ./...
```
