package favicon

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/png"
	"os"
	"testing"
	"testing/fstest"

	"github.com/forceu/gokapi/internal/test"
)

// generateTestImage creates a valid 512x512 PNG in memory for testing
func generateTestImage(t *testing.T) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 512, 512))
	buf := new(bytes.Buffer)
	err := png.Encode(buf, img)
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestInitAndGetFavicon(t *testing.T) {
	imageData := generateTestImage(t)

	// 1. Setup Mock FS for default icon
	mockFS := fstest.MapFS{
		"defaultFavicon.png": &fstest.MapFile{Data: imageData},
	}

	// 2. Setup a temporary file for the "custom" icon
	tmpFile, err := os.CreateTemp("", "custom_icon*.png")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	_, _ = tmpFile.Write(imageData)
	tmpFile.Close()

	t.Run("Initialize with default icon", func(t *testing.T) {
		// Pass a non-existent path to force use of fsDefault
		Init("non_existent_path.png", mockFS)

		// Verify various sizes
		icoRes := GetFavicon("/favicon.ico")
		test.IsEqualBool(t, len(icoRes) > 0, true)

		png16 := GetFavicon("/favicon-16x16.png")
		test.IsEqualBool(t, len(png16) > 0, true)

		png512 := GetFavicon("/favicon-android-chrome-512x512.png")
		test.IsEqualInt(t, len(png512), len(imageData))
	})

	t.Run("Initialize with custom icon", func(t *testing.T) {
		Init(tmpFile.Name(), mockFS)

		// Verify apple touch icon (180x180)
		appleIcon := GetFavicon("/favicon-apple-touch-icon.png")
		test.IsEqualBool(t, len(appleIcon) > 0, true)

		// Verify fallback to ICO
		fallback := GetFavicon("/unknown-path")
		test.IsEqualInt(t, len(fallback), len(faviconIco))
	})
}

func TestScaleImage(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 512, 512))

	t.Run("Scale to PNG", func(t *testing.T) {
		data := scaleImage(src, 32, true)
		img, err := png.Decode(bytes.NewReader(data))
		test.IsNil(t, err)
		test.IsEqualInt(t, img.Bounds().Dx(), 32)
		test.IsEqualInt(t, img.Bounds().Dy(), 32)
	})

	t.Run("Scale to ICO", func(t *testing.T) {
		data := scaleImage(src, 48, false)
		// Basic check for ICO header (00 00 01 00)
		test.IsEqualBool(t, len(data) > 4, true)
		test.IsEqualInt(t, int(data[2]), 1)
	})
}

// pngHeaderWithSize builds the signature and the header chunk of a PNG declaring the
// given dimensions. No pixel data follows, which is enough to read the dimensions and
// is what an image claiming to be enormous while weighing almost nothing looks like.
func pngHeaderWithSize(width, height uint32) []byte {
	data := []byte{0, 0, 0, 0, 0, 0, 0, 0, 8, 6, 0, 0, 0}
	binary.BigEndian.PutUint32(data[0:4], width)
	binary.BigEndian.PutUint32(data[4:8], height)

	chunk := append([]byte("IHDR"), data...)
	result := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}
	length := make([]byte, 4)
	binary.BigEndian.PutUint32(length, uint32(len(data)))
	result = append(result, length...)
	result = append(result, chunk...)
	crc := make([]byte, 4)
	binary.BigEndian.PutUint32(crc, crc32.ChecksumIEEE(chunk))
	return append(result, crc...)
}

func TestSetFromImage(t *testing.T) {
	t.Run("Refuses an oversized image without decoding it", func(t *testing.T) {
		before := faviconPng32x32
		err := SetFromImage(pngHeaderWithSize(20000, 20000))
		test.IsEqualBool(t, errors.Is(err, ErrIconTooLarge), true)
		// 20000 by 20000 pixels would take more than a gigabyte of memory to decode,
		// so reaching this line at all is the point of the test
		test.IsEqualBool(t, bytes.Equal(faviconPng32x32, before), true)
	})

	t.Run("Refuses a file that is not an image", func(t *testing.T) {
		err := SetFromImage([]byte("this is not an image"))
		test.IsNotNil(t, err)
	})

	t.Run("Accepts an image and replaces every size", func(t *testing.T) {
		var buffer bytes.Buffer
		test.IsNil(t, png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 256, 256))))
		test.IsNil(t, SetFromImage(buffer.Bytes()))

		for _, icon := range [][]byte{faviconPng16x16, faviconPng32x32, faviconPng180x180,
			faviconPng192x192, faviconPng512x512, faviconIco} {
			test.IsEqualBool(t, len(icon) > 0, true)
		}
		decoded, err := png.Decode(bytes.NewReader(faviconPng32x32))
		test.IsNil(t, err)
		test.IsEqualInt(t, decoded.Bounds().Dx(), 32)
	})
}
