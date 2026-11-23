package utils

import (
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

const (
	// MaxImageSize is the largest image we accept (5 MB).
	MaxImageSize = 5 << 20
	// MaxUploadBody caps a whole multipart request: one image plus text fields.
	// Handlers wrap r.Body with http.MaxBytesReader(w, r.Body, MaxUploadBody).
	MaxUploadBody = 6 << 20
	// MultipartMemory is how much of a multipart body Go keeps in RAM while
	// parsing; anything bigger is buffered in a temp file.
	MultipartMemory = 1 << 20
)

// UploadDir is where images are written, relative to the backend's working
// directory (backend/ locally, /app in Docker). The server serves this folder
// at /uploads/. It is a variable so tests can point it at a temp folder.
var UploadDir = "uploads"

// allowedImageTypes maps the type detected from the file's bytes to the
// extension we save it with. Anything not in this map is refused.
var allowedImageTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
}

var (
	ErrImageTooLarge = errors.New("image must be 5 MB or smaller")
	ErrImageType     = errors.New("image must be JPEG, PNG or GIF")
)

// SaveImage reads the uploaded file in form field `field`, checks it, and
// writes it to UploadDir under a random name.
//
// Call it after r.ParseMultipartForm. It returns:
//   - (nil, 0, nil) when no file was sent: images are always optional.
//   - (path, 0, nil) on success; path is the URL path, e.g. "/uploads/<uuid>.png",
//     which is what we store in the DB and send to the frontend.
//   - (nil, status, err) on failure; send err.Error() to the client with that
//     status (400, 413, 415 or 500).
func SaveImage(r *http.Request, field string) (*string, int, error) {
	file, header, err := r.FormFile(field)
	if errors.Is(err, http.ErrMissingFile) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, http.StatusBadRequest, errors.New("could not read image")
	}
	defer file.Close()

	// header.Size is counted by Go from the bytes actually received, not taken
	// from anything the client claims, so it is safe to trust.
	if header.Size > MaxImageSize {
		return nil, http.StatusRequestEntityTooLarge, ErrImageTooLarge
	}

	// Decide the type from the first 512 bytes (the file's "magic number"),
	// never from the filename or the Content-Type the browser sent: both are
	// chosen by the client and trivial to fake.
	head := make([]byte, 512)
	n, err := io.ReadFull(file, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, http.StatusBadRequest, errors.New("could not read image")
	}
	ext, ok := allowedImageTypes[http.DetectContentType(head[:n])]
	if !ok {
		return nil, http.StatusUnsupportedMediaType, ErrImageType
	}
	// Go back to the start so the whole file gets copied, not just bytes 512+.
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, http.StatusInternalServerError, errors.New("internal server error")
	}

	if err := os.MkdirAll(UploadDir, 0o755); err != nil {
		return nil, http.StatusInternalServerError, errors.New("internal server error")
	}

	// A random UUID name means the user can't pick the filename: no "../"
	// tricks, no overwriting someone else's image, and names can't be guessed.
	name := uuid.NewString() + ext
	diskPath := filepath.Join(UploadDir, name)

	// O_EXCL: fail instead of overwriting if the file somehow already exists.
	dst, err := os.OpenFile(diskPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return nil, http.StatusInternalServerError, errors.New("internal server error")
	}
	if _, err := io.Copy(dst, file); err != nil {
		dst.Close()
		os.Remove(diskPath)
		return nil, http.StatusInternalServerError, errors.New("internal server error")
	}
	if err := dst.Close(); err != nil {
		os.Remove(diskPath)
		return nil, http.StatusInternalServerError, errors.New("internal server error")
	}

	urlPath := "/uploads/" + name
	return &urlPath, 0, nil
}

// DeleteImage removes an image saved by SaveImage. Handlers call it when the
// DB insert fails after the image was saved, so no orphan files pile up.
// filepath.Base keeps only the file name, so it can never delete anything
// outside UploadDir. A nil path does nothing.
func DeleteImage(urlPath *string) {
	if urlPath == nil {
		return
	}
	name := filepath.Base(*urlPath)
	if err := os.Remove(filepath.Join(UploadDir, name)); err != nil && !os.IsNotExist(err) {
		log.Printf("DeleteImage %s: %v", name, err)
	}
}
