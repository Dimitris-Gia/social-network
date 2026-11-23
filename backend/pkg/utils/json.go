// Package utils holds small helpers shared by every handler.
// json.go: the only place that turns Go values into HTTP JSON responses
// and HTTP JSON request bodies into Go values.
package utils

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
)

// maxJSONBody caps how much a client may send us as JSON (1 MB).
// Without a cap, one request with a huge body could eat the server's memory.
// Image uploads use multipart/form-data and have their own limit in images.go.
const maxJSONBody = 1 << 20

// WriteJSON sends `data` as JSON with the given status code.
//
// We marshal BEFORE writing the header: if marshalling fails we can still
// send a clean 500 instead of a half-written 200 response.
func WriteJSON(w http.ResponseWriter, status int, data any) {
	body, err := json.Marshal(data)
	if err != nil {
		log.Printf("WriteJSON: marshal failed: %v", err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(body)
}

// WriteError sends {"error": "..."} so the frontend always gets the same shape.
//
// SECURITY: only pass messages written for the user ("email already in use"),
// never err.Error() from the database. SQL errors reveal table and column
// names. Log the real error with log.Printf and send a generic message.
func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, map[string]string{"error": message})
}

// ServerError logs the real error for us and sends the client a generic 500.
// Internal details (SQL errors, file paths) must never reach the browser.
func ServerError(w http.ResponseWriter, err error) {
	log.Printf("internal error: %v", err)
	WriteError(w, http.StatusInternalServerError, "internal server error")
}

// ReadJSON decodes the request body into dst (pass a pointer to a struct).
// The returned error is safe to show to the client, because it only
// describes what was wrong with THEIR input.
//
// Usage in a handler:
//
//	var req loginRequest
//	if err := utils.ReadJSON(w, r, &req); err != nil {
//		utils.WriteError(w, http.StatusBadRequest, err.Error())
//		return
//	}
func ReadJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)

	dec := json.NewDecoder(r.Body)
	// A typo in the frontend ("emial") becomes a clear 400 error
	// instead of a silently empty field.
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		var tooBig *http.MaxBytesError
		switch {
		case errors.Is(err, io.EOF):
			return errors.New("request body is empty")
		case errors.As(err, &tooBig):
			return errors.New("request body is too large")
		default:
			return errors.New("invalid JSON: " + err.Error())
		}
	}

	// Refuse bodies like {"a":1}{"b":2}: exactly one JSON value is allowed.
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain a single JSON object")
	}
	return nil
}
