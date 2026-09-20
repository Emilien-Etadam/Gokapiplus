// This file contains JS code for the counters above the upload box
// All files named admin_*.js will be merged together and minimised by calling
// go generate ./...
//
// The counters are rendered by the server, this file keeps them in sync while the
// page stays open: files are added after an upload and removed after a deletion
// without the page being reloaded.

const STATS_EXPIRING_SOON_SECONDS = 24 * 60 * 60;

// Registers the counters. Does nothing on the pages that do not show them.
function initAdminStats() {
    const container = document.getElementById("adminStats");
    const table = document.getElementById("downloadtable");
    if (container == null || table == null) {
        return;
    }
    // Any change to the file list recomputes the counters, whichever code caused it
    const observer = new MutationObserver(() => refreshAdminStats());
    observer.observe(table, {
        childList: true,
        subtree: true,
        characterData: true
    });
    refreshAdminStats();
}

function refreshAdminStats() {
    const table = document.getElementById("downloadtable");
    if (table == null) {
        return;
    }
    const rows = table.querySelectorAll("tr[id^='row-']");
    const now = Math.floor(Date.now() / 1000);

    let storageUsed = 0;
    let downloads = 0;
    let expiringSoon = 0;

    for (const row of rows) {
        // The size cell already carries the exact byte count for the sorting
        const sizeCell = row.querySelector("td[data-order]");
        if (sizeCell != null) {
            storageUsed = storageUsed + (parseInt(sizeCell.getAttribute("data-order"), 10) || 0);
        }
        const downloadCell = row.querySelector("td[id^='cell-downloads-']");
        if (downloadCell != null) {
            downloads = downloads + (parseInt(downloadCell.innerText, 10) || 0);
        }
        const expiry = parseInt(row.dataset.expire, 10) || 0;
        if (expiry > now && expiry <= now + STATS_EXPIRING_SOON_SECONDS) {
            expiringSoon++;
        }
    }

    setStatValue("statFileCount", rows.length);
    setStatValue("statStorageUsed", formatStatBytes(storageUsed));
    setStatValue("statDownloadCount", downloads);
    setStatValue("statExpiringSoon", expiringSoon);
}

function setStatValue(elementId, value) {
    const element = document.getElementById(elementId);
    if (element == null || element.innerText === String(value)) {
        return;
    }
    element.innerText = value;
}

// Same output as helper.ByteCountSI on the server side
function formatStatBytes(bytes) {
    const unit = 1024;
    if (bytes < unit) {
        return bytes + " B";
    }
    const units = ["k", "M", "G", "T", "P", "E"];
    let div = unit;
    let exp = 0;
    for (let n = Math.floor(bytes / unit); n >= unit; n = Math.floor(n / unit)) {
        div = div * unit;
        exp++;
    }
    return (bytes / div).toFixed(1) + " " + units[exp] + "B";
}
