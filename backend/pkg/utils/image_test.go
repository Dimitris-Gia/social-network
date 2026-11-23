package utils

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newUploadRequest builds a multipart request with one file field and parses
// it, the same way a handler would before calling SaveImage.
func newUploadRequest(t *testing.T, field, filename string, data []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if data != nil {
		fw, err := mw.CreateFormFile(field, filename)
		if err != nil {
			t.Fatal(err)
		}
		fw.Write(data)
	}
	mw.WriteField("first_name", "Maria")
	mw.Close()

	r := httptest.NewRequest(http.MethodPost, "/api/register", &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	if err := r.ParseMultipartForm(MultipartMemory); err != nil {
		t.Fatal(err)
	}
	return r
}

func encode(t *testing.T, format string) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(1, 1, color.RGBA{255, 0, 0, 255})
	var buf bytes.Buffer
	var err error
	switch format {
	case "png":
		err = png.Encode(&buf, img)
	case "jpeg":
		err = jpeg.Encode(&buf, img, nil)
	case "gif":
		err = gif.Encode(&buf, img, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestSaveImageAcceptsJPEGPNGGIF(t *testing.T) {
	UploadDir = t.TempDir()
	cases := map[string]string{"png": ".png", "jpeg": ".jpg", "gif": ".gif"}
	for format, ext := range cases {
		data := encode(t, format)
		// Deliberately misleading filename: the extension must come from the bytes.
		r := newUploadRequest(t, "avatar", "photo.html", data)
		path, status, err := SaveImage(r, "avatar")
		if err != nil || status != 0 || path == nil {
			t.Fatalf("%s: got path=%v status=%d err=%v", format, path, status, err)
		}
		if !strings.HasPrefix(*path, "/uploads/") || !strings.HasSuffix(*path, ext) {
			t.Fatalf("%s: unexpected path %q", format, *path)
		}
		saved, err := os.ReadFile(filepath.Join(UploadDir, filepath.Base(*path)))
		if err != nil || !bytes.Equal(saved, data) {
			t.Fatalf("%s: saved file differs from upload (err=%v)", format, err)
		}
	}
}

func TestSaveImageNoFileIsOptional(t *testing.T) {
	UploadDir = t.TempDir()
	r := newUploadRequest(t, "avatar", "", nil)
	path, status, err := SaveImage(r, "avatar")
	if path != nil || status != 0 || err != nil {
		t.Fatalf("got path=%v status=%d err=%v, want nil,0,nil", path, status, err)
	}
}

func TestSaveImageRejectsFakeImage(t *testing.T) {
	UploadDir = t.TempDir()
	html := []byte("<html><script>alert(document.cookie)</script></html>")
	r := newUploadRequest(t, "avatar", "cat.png", html) // .png name, HTML bytes
	path, status, err := SaveImage(r, "avatar")
	if path != nil || status != http.StatusUnsupportedMediaType || err != ErrImageType {
		t.Fatalf("got path=%v status=%d err=%v, want 415", path, status, err)
	}
	if entries, _ := os.ReadDir(UploadDir); len(entries) != 0 {
		t.Fatalf("rejected file was written to disk")
	}
}

func TestSaveImageRejectsTooLarge(t *testing.T) {
	UploadDir = t.TempDir()
	big := append(encode(t, "png"), make([]byte, MaxImageSize)...) // valid PNG header, > 5 MB
	r := newUploadRequest(t, "avatar", "big.png", big)
	_, status, err := SaveImage(r, "avatar")
	if status != http.StatusRequestEntityTooLarge || err != ErrImageTooLarge {
		t.Fatalf("got status=%d err=%v, want 413", status, err)
	}
}

func TestDeleteImageStaysInsideUploadDir(t *testing.T) {
	UploadDir = t.TempDir()
	outside := filepath.Join(t.TempDir(), "keep.txt")
	os.WriteFile(outside, []byte("x"), 0o644)

	evil := "/uploads/../../" + outside
	DeleteImage(&evil)
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("DeleteImage removed a file outside UploadDir")
	}
	DeleteImage(nil) // must not panic
}
