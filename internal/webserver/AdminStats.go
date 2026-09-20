package webserver

import (
	"time"

	"github.com/forceu/gokapi/internal/helper"
	"github.com/forceu/gokapi/internal/models"
)

// This file contains the counters shown above the upload box. They are methods on
// AdminView so that the template can call them without changing the view struct.
// The values are a snapshot of the file list at render time; the JS in
// admin_ui_stats.js keeps them in sync while the page is open.

// expiringSoonWindow is the period during which a file counts as expiring soon
const expiringSoonWindow = 24 * time.Hour

// StatFileCount returns the number of files that are currently shared
func (u *AdminView) StatFileCount() int {
	return len(u.Items)
}

// StatStorageUsed returns the size of all shared files in a human-readable format
func (u *AdminView) StatStorageUsed() string {
	var total int64
	for _, item := range u.Items {
		total = total + item.SizeBytes
	}
	return helper.ByteCountSI(total)
}

// StatDownloadCount returns how often the shared files have been downloaded in total
func (u *AdminView) StatDownloadCount() int {
	result := 0
	for _, item := range u.Items {
		result = result + item.DownloadCount
	}
	return result
}

// StatExpiringSoon returns the number of files that expire within the next 24 hours
func (u *AdminView) StatExpiringSoon() int {
	now := time.Now()
	limit := now.Add(expiringSoonWindow).Unix()
	result := 0
	for _, item := range u.Items {
		if item.UnlimitedTime {
			continue
		}
		if item.ExpireAt > now.Unix() && item.ExpireAt <= limit {
			result++
		}
	}
	return result
}

// fileStatus is the state badge shown in the file table
type fileStatus struct {
	Label string
	Class string
}

// FileStatus returns the state of a file for the table. Files that have expired or
// that have no download left are not listed at all, so those states cannot show up
// here. The same rules are applied in admin_ui_stats.js while the page is open.
func (u *AdminView) FileStatus(item models.FileApiOutput) fileStatus {
	if !item.UnlimitedTime && item.ExpireAt-time.Now().Unix() <= int64(expiringSoonWindow.Seconds()) {
		return fileStatus{Label: "Expires soon", Class: "bg-warning"}
	}
	if !item.UnlimitedDownloads && item.DownloadsRemaining == 1 {
		return fileStatus{Label: "Last download", Class: "bg-warning"}
	}
	if item.UnlimitedTime && item.UnlimitedDownloads {
		return fileStatus{Label: "Unlimited", Class: "bg-primary"}
	}
	return fileStatus{Label: "Active", Class: "bg-success"}
}
