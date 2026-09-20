package webserver

import (
	"path/filepath"
	"strings"
)

// This file contains the information shown to the recipient on the download page.

// IsArchive returns true if the shared file is a zip archive, which is what the
// automatic compression of a multi-file upload produces. The name of an
// end-to-end encrypted file is only known to the browser, so it never counts.
// The receiver is a value, because the template is executed with a DownloadView value
func (d DownloadView) IsArchive() bool {
	if d.EndToEndEncryption {
		return false
	}
	return strings.EqualFold(filepath.Ext(d.Name), ".zip")
}
