package webserver

import (
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/forceu/gokapi/internal/configuration"
	"github.com/forceu/gokapi/internal/helper"
	"github.com/forceu/gokapi/internal/webserver/authentication"
	"github.com/forceu/gokapi/internal/webserver/favicon"
)

// ViewBranding is the identifier for the appearance menu. It continues the
// View* constants declared in Webserver.go and is also used to highlight the menu
// entry in html_header.tmpl
const ViewBranding = 5

const (
	brandingFolderName     = "branding"
	brandingSettingsFile   = "settings.json"
	brandingLogoName       = "logo"
	brandingBackgroundName = "background"
	brandingFaviconName    = "favicon"
)

const (
	brandingModeDefault = "default"
	brandingModeColor   = "color"
	brandingModeImage   = "image"
)

const (
	maxLogoSizeBytes       = 1 * 1024 * 1024
	maxBackgroundSizeBytes = 5 * 1024 * 1024
	maxFaviconSizeBytes    = 1 * 1024 * 1024
	maxPublicNameLength    = 60
)

// allowedImageTypes maps an accepted file extension to the content type that the
// file has to be detected as. Anything else is rejected.
var allowedImageTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".webp": "image/webp",
	".svg":  "image/svg+xml",
}

// allowedPhotoTypes leaves out SVG, which is not an option for a background photograph
var allowedPhotoTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".webp": "image/webp",
}

// allowedIconTypes only holds PNG, as the server scales the icon itself and decodes no
// other format
var allowedIconTypes = map[string]string{
	".png": "image/png",
}

// brandingSettings contains everything that can be changed in the appearance menu.
// The background and the logo only affect the pages that recipients see. The browser
// tab icon applies everywhere, as it is part of the server rather than of a page.
type brandingSettings struct {
	BackgroundMode  string `json:"backgroundMode"`
	BackgroundColor string `json:"backgroundColor"`
	BackgroundFile  string `json:"backgroundFile"`
	AccentColor     string `json:"accentColor"`
	VeilPercent     int    `json:"veilPercent"`
	ShowLogo        bool   `json:"showLogo"`
	LogoFile        string `json:"logoFile"`
	LogoHeight      int    `json:"logoHeight"`
	FaviconFile     string `json:"faviconFile"`
}

// brandingView adds the settings to the regular admin view. The embedded pointer
// keeps all fields the header template expects available.
type brandingView struct {
	*AdminView
	Branding brandingSettings
	IsSaved  bool
	Error    string
}

func defaultBrandingSettings() brandingSettings {
	return brandingSettings{
		BackgroundMode:  brandingModeDefault,
		BackgroundColor: "#1f4f7a",
		AccentColor:     "#0d6efd",
		VeilPercent:     55,
		ShowLogo:        true,
		LogoHeight:      48,
	}
}

// brandingFolder returns the folder holding the branding settings and assets. It is
// located in the config folder, so that it is not served to the public.
func brandingFolder() string {
	return filepath.Join(configuration.GetEnvironment().ConfigDir, brandingFolderName)
}

func brandingFilePath(name string) string {
	return filepath.Join(brandingFolder(), name)
}

// loadBrandingSettings reads the stored settings. If none exist or they cannot be
// read, the defaults are returned, which leave the public pages unchanged.
func loadBrandingSettings() brandingSettings {
	content, err := os.ReadFile(brandingFilePath(brandingSettingsFile))
	if err != nil {
		return defaultBrandingSettings()
	}
	result := defaultBrandingSettings()
	err = json.Unmarshal(content, &result)
	if err != nil {
		return defaultBrandingSettings()
	}
	return result
}

func saveBrandingSettings(settings brandingSettings) error {
	err := os.MkdirAll(brandingFolder(), 0700)
	if err != nil {
		return err
	}
	content, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(brandingFilePath(brandingSettingsFile), content, 0600)
}

// showBranding renders the appearance menu
func showBranding(w http.ResponseWriter, r *http.Request) {
	user, err := authentication.GetUserFromRequest(r)
	if err != nil {
		panic(err)
	}
	view := (&AdminView{}).convertGlobalConfig(ViewBranding, user)
	if !view.ActiveUser.IsSuperAdmin() {
		redirect(w, r, "admin")
		return
	}
	err = templateFolder.ExecuteTemplate(w, "branding", brandingView{
		AdminView: view,
		Branding:  loadBrandingSettings(),
		IsSaved:   r.URL.Query().Get("saved") == "1",
		Error:     sanitiseBrandingError(r.URL.Query().Get("error"))})
	helper.CheckIgnoreTimeout(err)
}

// sanitiseBrandingError makes sure that only known messages can be shown, as the
// value is passed in the URL
func sanitiseBrandingError(errorId string) string {
	switch errorId {
	case "logo":
		return "Le logo n'a pas pu être enregistré. Utilisez un fichier PNG, JPG, WEBP ou SVG de 1 Mo maximum."
	case "background":
		return "L'image de fond n'a pas pu être enregistrée. Utilisez un fichier PNG, JPG ou WEBP de 5 Mo maximum."
	case "favicon":
		return "L'icône de l'onglet n'a pas pu être enregistrée. Utilisez une image PNG carrée de 1 Mo maximum."
	case "name":
		return "Le nom affiché n'a pas pu être enregistré dans la configuration du serveur."
	case "write":
		return "Les réglages n'ont pas pu être écrits sur le disque. Vérifiez les droits du dossier de configuration."
	default:
		return ""
	}
}

// saveBranding stores the submitted appearance settings and writes the stylesheet
// that the public pages load
func saveBranding(w http.ResponseWriter, r *http.Request) {
	user, err := authentication.GetUserFromRequest(r)
	if err != nil {
		panic(err)
	}
	if !user.IsSuperAdmin() {
		redirect(w, r, "admin")
		return
	}
	err = r.ParseMultipartForm(maxBackgroundSizeBytes)
	if err != nil {
		redirectAfterSave(w, r, "branding?error=background")
		return
	}

	settings := loadBrandingSettings()
	settings.BackgroundMode = parseBrandingMode(r.FormValue("backgroundMode"))
	settings.BackgroundColor = parseHexColor(r.FormValue("backgroundColor"), settings.BackgroundColor)
	settings.AccentColor = parseHexColor(r.FormValue("accentColor"), settings.AccentColor)
	settings.VeilPercent = parseBoundedInt(r.FormValue("veilPercent"), 0, 90, settings.VeilPercent)
	settings.LogoHeight = parseBoundedInt(r.FormValue("logoHeight"), 20, 120, settings.LogoHeight)
	settings.ShowLogo = r.FormValue("showLogo") == "1"

	if r.FormValue("removeLogo") == "1" {
		removeBrandingAsset(settings.LogoFile)
		settings.LogoFile = ""
	}
	if r.FormValue("removeBackground") == "1" {
		removeBrandingAsset(settings.BackgroundFile)
		settings.BackgroundFile = ""
	}
	if r.FormValue("removeFavicon") == "1" {
		removeBrandingAsset(settings.FaviconFile)
		settings.FaviconFile = ""
	}

	newLogo, err := storeUploadedImage(r, "logo", brandingLogoName, maxLogoSizeBytes, allowedImageTypes)
	if err != nil {
		redirectAfterSave(w, r, "branding?error=logo")
		return
	}
	if newLogo != "" {
		if newLogo != settings.LogoFile {
			removeBrandingAsset(settings.LogoFile)
		}
		settings.LogoFile = newLogo
	}

	newBackground, err := storeUploadedImage(r, "background", brandingBackgroundName, maxBackgroundSizeBytes, allowedPhotoTypes)
	if err != nil {
		redirectAfterSave(w, r, "branding?error=background")
		return
	}
	if newBackground != "" {
		if newBackground != settings.BackgroundFile {
			removeBrandingAsset(settings.BackgroundFile)
		}
		settings.BackgroundFile = newBackground
	}

	newFavicon, err := storeUploadedImage(r, "favicon", brandingFaviconName, maxFaviconSizeBytes, allowedIconTypes)
	if err != nil {
		redirectAfterSave(w, r, "branding?error=favicon")
		return
	}
	if newFavicon != "" {
		settings.FaviconFile = newFavicon
	}

	err = saveBrandingSettings(settings)
	if err != nil {
		redirectAfterSave(w, r, "branding?error=write")
		return
	}
	// The icon is held in memory, so it has to be rebuilt for the change to show
	err = applyBrandingFavicon(settings)
	if err != nil {
		redirectAfterSave(w, r, "branding?error=favicon")
		return
	}
	err = savePublicName(r.FormValue("publicName"))
	if err != nil {
		redirectAfterSave(w, r, "branding?error=name")
		return
	}
	redirectAfterSave(w, r, "branding?saved=1")
}

// redirectAfterSave sends the browser back to the menu with a GET request, so that the
// form is not submitted again when the page is reloaded
func redirectAfterSave(w http.ResponseWriter, r *http.Request, url string) {
	http.Redirect(w, r, url, http.StatusSeeOther)
}

func parseBrandingMode(value string) string {
	switch value {
	case brandingModeColor, brandingModeImage:
		return value
	default:
		return brandingModeDefault
	}
}

// parseHexColor only accepts the #rrggbb notation, as the value ends up in a stylesheet
func parseHexColor(value, fallback string) string {
	if len(value) != 7 || value[0] != '#' {
		return fallback
	}
	for _, char := range value[1:] {
		isHex := (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')
		if !isHex {
			return fallback
		}
	}
	return strings.ToLower(value)
}

func parseBoundedInt(value string, min, max, fallback int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < min || parsed > max {
		return fallback
	}
	return parsed
}

// storeUploadedImage saves an uploaded image and returns its new file name. An empty
// name is returned if no file was submitted.
func storeUploadedImage(r *http.Request, formField, targetName string, maxSize int64, allowed map[string]string) (string, error) {
	file, header, err := r.FormFile(formField)
	if err != nil {
		return "", nil // no file submitted, keep the current one
	}
	defer file.Close()
	if header.Size > maxSize {
		return "", os.ErrInvalid
	}
	extension := strings.ToLower(filepath.Ext(header.Filename))
	expectedType, isAllowed := allowed[extension]
	if !isAllowed {
		return "", os.ErrInvalid
	}
	err = verifyImageContent(file, extension, expectedType)
	if err != nil {
		return "", err
	}
	err = os.MkdirAll(brandingFolder(), 0700)
	if err != nil {
		return "", err
	}
	fileName := targetName + extension
	target, err := os.OpenFile(brandingFilePath(fileName), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return "", err
	}
	defer target.Close()
	_, err = io.Copy(target, io.LimitReader(file, maxSize))
	if err != nil {
		return "", err
	}
	return fileName, nil
}

// verifyImageContent checks that the content of the file matches its extension, so
// that no arbitrary content can be served from the branding folder
func verifyImageContent(file multipart.File, extension, expectedType string) error {
	buffer := make([]byte, 512)
	read, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return err
	}
	_, err = file.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}
	content := buffer[:read]
	if extension == ".svg" {
		// SVG is XML and is not reliably detected, therefore the root element is looked up
		if !strings.Contains(strings.ToLower(string(content)), "<svg") {
			return os.ErrInvalid
		}
		return nil
	}
	if http.DetectContentType(content) != expectedType {
		return os.ErrInvalid
	}
	return nil
}

// savePublicName stores the name shown on the pages. An empty or overlong name is
// ignored, which leaves the current name in place.
func savePublicName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > maxPublicNameLength {
		return nil
	}
	if name == configuration.Get().PublicName {
		return nil
	}
	return configuration.SetPublicName(name)
}

// applyBrandingFavicon rebuilds the icon that the browser tab shows. The default icon
// is restored when no custom one is set.
func applyBrandingFavicon(settings brandingSettings) error {
	if settings.FaviconFile == "" {
		favicon.Init(pathCustomFavicon, staticFolder)
		return nil
	}
	content, err := os.ReadFile(brandingFilePath(settings.FaviconFile))
	if err != nil {
		return err
	}
	return favicon.SetFromImage(content)
}

func removeBrandingAsset(fileName string) {
	if fileName == "" {
		return
	}
	_ = os.Remove(brandingFilePath(fileName))
}

// brandingAssetVersion returns the modification time of an asset, which is appended to
// its URL so that browsers pick up a replaced image
func brandingAssetVersion(fileName string) string {
	info, err := os.Stat(brandingFilePath(fileName))
	if err != nil {
		return "0"
	}
	return strconv.FormatInt(info.ModTime().Unix(), 10)
}

func generateBrandingCss(settings brandingSettings) string {
	var css strings.Builder
	css.WriteString("/* Generated by Gokapi from the appearance settings. Do not edit by hand. */\n")

	veil := ""
	if settings.VeilPercent > 0 {
		// The veil is part of the background shorthand instead of an overlay element,
		// so that it cannot interfere with the existing page layout
		veil = "linear-gradient(rgba(0,0,0," + formatOpacity(settings.VeilPercent) + "),rgba(0,0,0," + formatOpacity(settings.VeilPercent) + "))"
	}

	switch settings.BackgroundMode {
	case brandingModeColor:
		css.WriteString("body{background-color:" + settings.BackgroundColor + "!important;")
		if veil != "" {
			css.WriteString("background-image:" + veil + "!important;")
		} else {
			css.WriteString("background-image:none!important;")
		}
		css.WriteString("}\n")
	case brandingModeImage:
		if settings.BackgroundFile == "" {
			break
		}
		imageUrl := "url(\"./branding/asset/background?v=" + brandingAssetVersion(settings.BackgroundFile) + "\")"
		image := imageUrl
		if veil != "" {
			image = veil + "," + imageUrl
		}
		css.WriteString("body{background-color:" + settings.BackgroundColor + "!important;")
		css.WriteString("background-image:" + image + "!important;")
		css.WriteString("background-size:cover!important;background-position:center!important;")
		css.WriteString("background-repeat:no-repeat!important;background-attachment:fixed!important;}\n")
	}

	if settings.ShowLogo && settings.LogoFile != "" {
		logoUrl := "url(\"./branding/asset/logo?v=" + brandingAssetVersion(settings.LogoFile) + "\")"
		// The logo replaces the site name in the public header instead of floating above
		// the page. Both used to be drawn at the top centre, so they overlapped. Sitting
		// in the normal flow also keeps the logo clickable and lets it scroll with the page.
		css.WriteString(".gk-sitetitle{height:" + strconv.Itoa(settings.LogoHeight) + "px;")
		css.WriteString("margin:0;font-size:0;line-height:0;")
		css.WriteString("background:" + logoUrl + " center/contain no-repeat;}\n")
		css.WriteString(".gk-sitetitle a{display:block;height:100%;}\n")
	}

	if settings.AccentColor != "" {
		css.WriteString(".btn-primary{background-color:" + settings.AccentColor + "!important;")
		css.WriteString("border-color:" + settings.AccentColor + "!important;}\n")
		css.WriteString(".btn-primary:hover,.btn-primary:focus{filter:brightness(1.08);}\n")
	}
	return css.String()
}

// formatOpacity converts a percentage into the decimal notation used in rgba()
func formatOpacity(percent int) string {
	return strconv.FormatFloat(float64(percent)/100, 'f', 2, 64)
}

// serveBrandingCss serves the generated stylesheet to the public pages. It is built
// from the stored settings on every request, so that upgrading Gokapi can never leave
// a stale stylesheet behind. It stays empty as long as nothing has been customised.
func serveBrandingCss(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(generateBrandingCss(loadBrandingSettings())))
}

// serveBrandingAsset serves the logo or the background image. Only these two names are
// accepted and the file name is read from the settings, never from the URL.
func serveBrandingAsset(w http.ResponseWriter, r *http.Request) {
	settings := loadBrandingSettings()
	var fileName string
	switch r.PathValue("name") {
	case brandingLogoName:
		fileName = settings.LogoFile
	case brandingBackgroundName:
		fileName = settings.BackgroundFile
	}
	if fileName == "" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=100800") // 2 days, the URL contains a version
	http.ServeFile(w, r, brandingFilePath(fileName))
}
