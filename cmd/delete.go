package cmd

import "os"

func canDelete(item fileItem) bool {
	return item.snapshot != nil && item.snapshot.Mode().IsRegular() &&
		item.status != statusMoved && item.status != statusDeleted
}

func deleteItem(item fileItem) fileItem {
	if !canDelete(item) {
		return item
	}
	if err := verifySource(item); err != nil {
		item.status, item.reason = statusFailed, err.Error()
		return item
	}
	if err := os.Remove(item.source); err != nil {
		item.status, item.reason = statusFailed, err.Error()
		return item
	}
	item.destination = ""
	item.status, item.reason = statusDeleted, "Permanently deleted; cannot be restored by this tool"
	return item
}
