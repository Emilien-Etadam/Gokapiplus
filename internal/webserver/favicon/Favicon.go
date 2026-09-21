package favicon

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"io/fs"
	"math"
	"os"
	"strings"

	"github.com/Kodeworks/golang-image-ico"
	"github.com/forceu/gokapi/internal/helper"
	"golang.org/x/image/draw"
)

// maxIconDimension bounds the size of an icon that is set while the server runs. An
// image is decoded into memory in full, so a small file declaring huge dimensions would
// exhaust the memory of the server long before anything is scaled down.
const maxIconDimension = 2048

// ErrIconTooLarge is returned for an image whose dimensions exceed maxIconDimension
var ErrIconTooLarge = errors.New("the icon is larger than 2048 pixels on a side")

var faviconIco []byte

var faviconPng16x16 []byte

var faviconPng32x32 []byte
var faviconPng180x180 []byte

var faviconPng192x192 []byte

var faviconPng512x512 []byte

// Init creates the favicon with a custom icon or the default one
func Init(pathCustomIcon string, fsDefault fs.FS) {
	var imageContent []byte
	exists, err := helper.FileExists(pathCustomIcon)
	helper.Check(err)
	if exists {
		content, err := os.ReadFile(pathCustomIcon)
		helper.Check(err)
		imageContent = content
	} else {
		content, err := fsDefault.Open("defaultFavicon.png")
		helper.Check(err)
		defer content.Close()
		imageContent, err = io.ReadAll(content)
		helper.Check(err)
	}
	img, _, err := image.Decode(bytes.NewReader(imageContent))
	if err != nil {
		fmt.Println(err)
		fmt.Println("Could not decode favicon, please make sure to supply a 512x512 png image.")
		os.Exit(1)
	}
	bounds := img.Bounds()
	if bounds.Dx() != 512 || bounds.Dy() != 512 {
		fmt.Println("Could not decode favicon, please make sure to supply a 512x512 png image.")
		os.Exit(1)
	}

	faviconIco = scaleImage(img, 48, false)

	faviconPng16x16 = scaleImage(img, 16, true)
	faviconPng32x32 = scaleImage(img, 32, true)
	faviconPng180x180 = scaleImage(img, 180, true)
	faviconPng192x192 = scaleImage(img, 192, true)
	faviconPng512x512 = imageContent
}

// SetFromImage replaces the favicon with the given image, scaled to every size that is
// served. Unlike Init it returns an error instead of ending the program, so that it can
// be called while the server is running. Only PNG images can be decoded.
func SetFromImage(content []byte) error {
	// The dimensions are read from the header first, as decoding the image allocates
	// width by height pixels whatever the size of the file
	config, _, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil {
		return err
	}
	if config.Width > maxIconDimension || config.Height > maxIconDimension {
		return ErrIconTooLarge
	}
	img, _, err := image.Decode(bytes.NewReader(content))
	if err != nil {
		return err
	}
	faviconIco = scaleImage(img, 48, false)
	faviconPng16x16 = scaleImage(img, 16, true)
	faviconPng32x32 = scaleImage(img, 32, true)
	faviconPng180x180 = scaleImage(img, 180, true)
	faviconPng192x192 = scaleImage(img, 192, true)
	faviconPng512x512 = scaleImage(img, 512, true)
	return nil
}

// GetFavicon returns the favicon for the given url
func GetFavicon(url string) []byte {
	if strings.HasPrefix(url, "/favicon.ico") {
		return faviconIco
	}
	if strings.HasPrefix(url, "/favicon-16x16.png") {
		return faviconPng16x16
	}
	if strings.HasPrefix(url, "/favicon-32x32.png") {
		return faviconPng32x32
	}
	if strings.HasPrefix(url, "/favicon-android-chrome-192x192.png") {
		return faviconPng192x192
	}
	if strings.HasPrefix(url, "/favicon-android-chrome-512x512.png") {
		return faviconPng512x512
	}
	if strings.HasPrefix(url, "/favicon-apple-touch-icon.png") {
		return faviconPng180x180
	}
	return faviconIco
}

func scaleImage(src image.Image, size int, isPng bool) []byte {
	buf := new(bytes.Buffer)
	ratio := (float64)(src.Bounds().Max.Y) / (float64)(src.Bounds().Max.X)
	height := int(math.Round(float64(size) * ratio))
	dst := image.NewRGBA(image.Rect(0, 0, size, height))
	draw.NearestNeighbor.Scale(dst, dst.Rect, src, src.Bounds(), draw.Over, nil)

	if isPng {
		err := png.Encode(buf, dst)
		helper.Check(err)
		return buf.Bytes()
	}
	err := ico.Encode(buf, dst)
	helper.Check(err)
	return buf.Bytes()
}
