// This file contains JS code for the appearance menu
// All files named admin_*.js will be merged together and minimised by calling
// go generate ./...
//
// The menu writes the branding to the server, this file only keeps the preview in
// sync with the form while it is being edited.

var brandingObjectUrls = [];

// Registers the preview of the appearance menu. Does nothing on the other admin pages.
function initBrandingSettings() {
    const preview = document.getElementById("brandingPreview");
    if (preview == null) {
        return;
    }
    const form = document.getElementById("brandingForm");
    // Images that are already stored on the server, empty if none were uploaded yet
    const storedLogo = preview.getAttribute("data-logo-url");
    const storedBackground = preview.getAttribute("data-background-url");

    const state = {
        logo: storedLogo,
        background: storedBackground
    };

    const update = () => updateBrandingPreview(state);

    form.addEventListener("input", update);
    form.addEventListener("change", update);

    document.getElementById("logo").addEventListener("change", function() {
        state.logo = createBrandingObjectUrl(this.files) || storedLogo;
        update();
    });
    document.getElementById("background").addEventListener("change", function() {
        state.background = createBrandingObjectUrl(this.files) || storedBackground;
        update();
    });

    update();
}

// Creates a temporary URL for a selected file, so that it can be shown before upload
function createBrandingObjectUrl(files) {
    if (files == null || files.length === 0) {
        return "";
    }
    const url = URL.createObjectURL(files[0]);
    brandingObjectUrls.push(url);
    return url;
}

function updateBrandingPreview(state) {
    const preview = document.getElementById("brandingPreview");
    const logoPreview = document.getElementById("previewLogo");
    const previewButton = document.getElementById("previewButton");

    const mode = document.getElementById("backgroundMode").value;
    const backgroundColor = document.getElementById("backgroundColor").value;
    const accentColor = document.getElementById("accentColor").value;
    const veil = parseInt(document.getElementById("veilPercent").value, 10) / 100;
    const logoHeight = parseInt(document.getElementById("logoHeight").value, 10);
    const showLogo = document.getElementById("showLogo").checked;

    document.getElementById("veilValue").innerText = Math.round(veil * 100);
    document.getElementById("logoHeightValue").innerText = logoHeight;

    // The overlay is part of the background, the same way the generated stylesheet does it
    const overlay = veil > 0 ? `linear-gradient(rgba(0,0,0,${veil}),rgba(0,0,0,${veil}))` : "";
    let backgroundImage = "none";
    let color = "#212529";

    if (mode === "color") {
        color = backgroundColor;
        backgroundImage = overlay === "" ? "none" : overlay;
    }
    if (mode === "image") {
        color = backgroundColor;
        const image = isBrandingAssetRemoved("removeBackground") ? "" : state.background;
        if (image !== "" && image != null) {
            backgroundImage = overlay === "" ? `url("${image}")` : `${overlay},url("${image}")`;
        } else if (overlay !== "") {
            backgroundImage = overlay;
        }
    }

    preview.style.backgroundColor = color;
    preview.style.backgroundImage = backgroundImage;

    const logo = isBrandingAssetRemoved("removeLogo") ? "" : state.logo;
    if (showLogo && logo !== "" && logo != null) {
        logoPreview.style.backgroundImage = `url("${logo}")`;
        logoPreview.style.height = logoHeight + "px";
        logoPreview.style.display = "block";
    } else {
        logoPreview.style.display = "none";
    }

    previewButton.style.backgroundColor = accentColor;
    previewButton.style.borderColor = accentColor;
}

// Returns true if the checkbox that removes an image on save is checked
function isBrandingAssetRemoved(checkboxId) {
    const checkbox = document.getElementById(checkboxId);
    return checkbox != null && checkbox.checked;
}
