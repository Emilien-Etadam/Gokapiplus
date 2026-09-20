// This file contains JS code for the automatic client-side zipping of uploads
// All files named admin_*.js will be merged together and minimised by calling
// go generate ./...
//
// Whenever more than one file or a folder is dropped or selected, the selection is
// zipped in the browser and the resulting archive is passed to the regular upload
// pipeline as if it were a single file. A selection of exactly one file is never
// touched and keeps the original behaviour.
//
// The code is split into two layers:
//   1. The core layer, which contains all the logic and does not know anything about
//      the DOM, Dropzone or the current markup. It can be reused as-is.
//   2. The UI layer at the bottom of this file, which is a thin adapter connecting the
//      core layer to the upload UI currently in use. Only this layer has to be replaced
//      when the admin interface is rewritten.


// ############################## Core layer ##############################

// Maximum total size of a selection that will be zipped. Larger selections are rejected
// with an error instead of being compressed, as the whole archive is held in memory.
const ZIP_MAX_TOTAL_SIZE_BYTES = 2 * 1024 * 1024 * 1024;

// DEFLATE compression level used for the generated archive, 1 being the fastest and
// 9 the one resulting in the smallest archive
const ZIP_COMPRESSION_LEVEL = 6;


// Returns the name of the archive for the given date, e.g. partage-2026-09-20-14-05.zip
function zipGenerateName(date) {
    const pad = (value) => String(value).padStart(2, "0");
    return "partage-" + date.getFullYear() + "-" + pad(date.getMonth() + 1) + "-" + pad(date.getDate()) +
        "-" + pad(date.getHours()) + "-" + pad(date.getMinutes()) + ".zip";
}

// Reads all entries of a directory. readEntries() only returns a limited amount of
// entries per call (100 in Chromium), therefore it has to be called repeatedly until
// it returns an empty array.
function zipReadAllDirectoryEntries(reader) {
    return new Promise((resolve, reject) => {
        const allEntries = [];
        const readBatch = () => {
            reader.readEntries((entries) => {
                if (entries.length === 0) {
                    resolve(allEntries);
                    return;
                }
                for (const entry of entries) {
                    allEntries.push(entry);
                }
                readBatch();
            }, reject);
        };
        readBatch();
    });
}

// Converts a FileSystemFileEntry into a File object
function zipEntryToFile(fileEntry) {
    return new Promise((resolve, reject) => {
        fileEntry.file(resolve, reject);
    });
}

// Recursively walks a FileSystemEntry and appends all contained files to the result
// array, keeping the relative path of each file inside the dropped folder
async function zipWalkEntry(entry, parentPath, result) {
    if (entry.isFile) {
        const file = await zipEntryToFile(entry);
        result.push({
            file: file,
            path: parentPath + entry.name,
            isDirectory: false
        });
        return;
    }
    if (!entry.isDirectory) {
        return;
    }
    const childEntries = await zipReadAllDirectoryEntries(entry.createReader());
    if (childEntries.length === 0) {
        // Empty folders are stored as well, so that the original structure is preserved
        result.push({
            file: null,
            path: parentPath + entry.name + "/",
            isDirectory: true
        });
        return;
    }
    for (const childEntry of childEntries) {
        await zipWalkEntry(childEntry, parentPath + entry.name + "/", result);
    }
}

// Converts an array containing FileSystemEntry and/or File objects into a flat list of
// items that can be added to an archive. Folders are expanded recursively.
async function zipCollectItems(entries) {
    const items = [];
    for (const entry of entries) {
        if (entry instanceof File) {
            items.push({
                file: entry,
                // webkitRelativePath is set when a folder was selected with a file input
                path: entry.webkitRelativePath != null && entry.webkitRelativePath !== "" ? entry.webkitRelativePath : entry.name,
                isDirectory: false
            });
            continue;
        }
        await zipWalkEntry(entry, "", items);
    }
    return items;
}

// Renames items that would end up with an identical path inside the archive by adding
// a suffix before the file extension, e.g. image.jpg, image_1.jpg, image_2.jpg
function zipDeduplicateNames(items) {
    const usedPaths = new Set();
    for (const item of items) {
        if (!usedPaths.has(item.path)) {
            usedPaths.add(item.path);
            continue;
        }
        const separatorIndex = item.path.lastIndexOf("/");
        const directory = separatorIndex === -1 ? "" : item.path.substring(0, separatorIndex + 1);
        const name = item.path.substring(separatorIndex + 1);
        // A dot at the first position belongs to a hidden file and is not an extension
        const dotIndex = name.lastIndexOf(".");
        const baseName = dotIndex > 0 ? name.substring(0, dotIndex) : name;
        const extension = dotIndex > 0 ? name.substring(dotIndex) : "";
        let counter = 1;
        let newPath;
        do {
            newPath = directory + baseName + "_" + counter + extension;
            counter++;
        } while (usedPaths.has(newPath));
        item.path = newPath;
        usedPaths.add(newPath);
    }
    return items;
}

// Returns the sum of the size of all files of the passed items
function zipGetTotalSize(items) {
    let totalSize = 0;
    for (const item of items) {
        if (item.file != null) {
            totalSize = totalSize + item.file.size;
        }
    }
    return totalSize;
}

// Creates the archive and returns it as a File object, so that it can be uploaded like
// any regular file. onProgress is called with the current compression percentage.
function zipCreateArchive(items, archiveName, onProgress) {
    const archive = new JSZip();
    for (const item of items) {
        if (item.isDirectory) {
            archive.folder(item.path);
            continue;
        }
        archive.file(item.path, item.file, {
            date: new Date(item.file.lastModified)
        });
    }
    return archive.generateAsync({
        type: "blob",
        compression: "DEFLATE",
        compressionOptions: {
            level: ZIP_COMPRESSION_LEVEL
        }
    }, (metadata) => {
        onProgress(metadata.percent);
    }).then((blob) => new File([blob], archiveName, {
        type: "application/zip",
        lastModified: Date.now()
    }));
}

// Converts a size in bytes into a human readable string
function zipFormatSize(bytes) {
    const units = ["B", "KB", "MB", "GB"];
    let size = bytes;
    let unitIndex = 0;
    while (size >= 1024 && unitIndex < units.length - 1) {
        size = size / 1024;
        unitIndex++;
    }
    return (Math.round(size * 10) / 10) + units[unitIndex];
}


// ############################### UI layer ###############################

var isCompressing = false;

// Registers the interception of file drops and file selections. Has to be called after
// initDropzone() and only on pages that contain an upload box.
function initZipUpload() {
    // Both listeners are attached to the document in the capture phase, which means that
    // they are executed before the handlers Dropzone attached to the upload box. This
    // allows cancelling the regular handling for selections that need to be zipped.
    document.addEventListener("drop", zipOnDrop, true);
    document.addEventListener("change", zipOnFileInputChange, true);

    window.addEventListener("beforeunload", (event) => {
        if (isCompressing) {
            event.returnValue = "Files are still being compressed. Do you want to close this page?";
        }
    });
}

// Returns true if the upload box is currently able to accept files
function zipIsUploadPossible() {
    // If JSZip could not be loaded, all selections are passed on unmodified
    if (typeof JSZip === "undefined") {
        return false;
    }
    if (typeof dropzoneObject === "undefined" || dropzoneObject == null) {
        return false;
    }
    // The upload box is disabled while end-to-end encryption is still being set up
    if (dropzoneObject.disabled || isCompressing) {
        return false;
    }
    return true;
}

function zipOnDrop(event) {
    if (!zipIsUploadPossible() || event.dataTransfer == null) {
        return;
    }
    if (dropzoneObject.element == null || !dropzoneObject.element.contains(event.target)) {
        return;
    }

    // The items of a DataTransfer object are only accessible while the event is being
    // handled, therefore all entries have to be read synchronously before any
    // asynchronous work is started
    const entries = [];
    let containsDirectory = false;
    if (event.dataTransfer.items != null && event.dataTransfer.items.length > 0) {
        for (const item of event.dataTransfer.items) {
            if (item.kind !== "file") {
                continue;
            }
            const entry = typeof item.webkitGetAsEntry === "function" ? item.webkitGetAsEntry() : null;
            if (entry == null) {
                // Fallback for browsers that do not support the FileSystem API
                const file = item.getAsFile();
                if (file != null) {
                    entries.push(file);
                }
                continue;
            }
            if (entry.isDirectory) {
                containsDirectory = true;
            }
            entries.push(entry);
        }
    } else {
        for (const file of event.dataTransfer.files) {
            entries.push(file);
        }
    }

    // A single file without any folder is uploaded as-is, the event is passed on to Dropzone
    if (!containsDirectory && entries.length < 2) {
        return;
    }

    event.preventDefault();
    event.stopPropagation();
    // Dropzone's drop handler, which usually removes the highlighting of the upload box,
    // has been skipped and therefore the style has to be removed manually
    dropzoneObject.element.classList.remove("dz-drag-hover");

    zipCollectItems(entries)
        .then((items) => zipAndUpload(items))
        .catch((error) => zipShowGenericError(error));
}

function zipOnFileInputChange(event) {
    if (!zipIsUploadPossible()) {
        return;
    }
    // Only the hidden input that Dropzone uses for its file selection dialog is intercepted
    if (event.target !== dropzoneObject.hiddenFileInput) {
        return;
    }
    // The FileList is emptied further below, so the files have to be copied first
    const files = Array.from(event.target.files);
    if (files.length < 2) {
        return;
    }

    event.preventDefault();
    event.stopPropagation();
    // Dropzone replaces the input after every selection to be able to select the same
    // files again. As its handler is skipped, the input is reset here instead.
    event.target.value = "";

    zipCollectItems(files)
        .then((items) => zipAndUpload(items))
        .catch((error) => zipShowGenericError(error));
}

// Compresses the passed items and hands the resulting archive over to the regular upload
async function zipAndUpload(items) {
    const deduplicatedItems = zipDeduplicateNames(items);
    const archiveName = zipGenerateName(new Date());
    const statusId = getUuid();
    addFileStatus(statusId, archiveName);

    const totalSize = zipGetTotalSize(deduplicatedItems);
    if (totalSize > ZIP_MAX_TOTAL_SIZE_BYTES) {
        zipShowStatusError(statusId, "Error: Selection of " + zipFormatSize(totalSize) +
            " exceeds the maximum size of " + zipFormatSize(ZIP_MAX_TOTAL_SIZE_BYTES) + " for automatic compression");
        return;
    }

    isCompressing = true;
    let lastPercentage = -1;
    try {
        const archive = await zipCreateArchive(deduplicatedItems, archiveName, (percentage) => {
            const rounded = Math.round(percentage);
            // The callback is called very frequently, the UI is only updated on change
            if (rounded !== lastPercentage) {
                lastPercentage = rounded;
                zipUpdateStatus(statusId, rounded);
            }
        });
        removeFileStatus(statusId);
        // From here on the archive is treated like any other file that was added
        dropzoneObject.addFile(archive);
    } catch (error) {
        console.log(error);
        zipShowStatusError(statusId, "Error while compressing: " + error);
    } finally {
        isCompressing = false;
    }
}

// Displays the compression progress in the regular upload status area
function zipUpdateStatus(statusId, percentage) {
    const progressBar = document.getElementById(`us-progressbar-${statusId}`);
    const progressInfo = document.getElementById(`us-progress-info-${statusId}`);
    if (progressBar == null || progressInfo == null) {
        return;
    }
    progressBar.style.width = percentage + "%";
    progressInfo.innerText = "Compression: " + percentage + "%";
}

function zipShowStatusError(statusId, message) {
    // showError() only requires the id of the status entry to display the message
    showError({
        upload: {
            uuid: statusId
        }
    }, message);
}

function zipShowGenericError(error) {
    console.log(error);
    const statusId = getUuid();
    addFileStatus(statusId, "Compression");
    zipShowStatusError(statusId, "Error while reading the selected files: " + error);
}
