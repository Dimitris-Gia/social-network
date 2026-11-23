package handlers

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"social-network/pkg/utils"
)

// These cases are rejected before the database is touched, so a Handler with
// no DB is enough.
func TestRegisterRejectsBeforeDB(t *testing.T) {
	h := &Handler{}

	// 1. Body over 6 MB -> 413
	var big bytes.Buffer
	mw := multipart.NewWriter(&big)
	fw, _ := mw.CreateFormFile("avatar", "a.png")
	fw.Write(make([]byte, utils.MaxUploadBody+1))
	mw.Close()
	r := httptest.NewRequest(http.MethodPost, "/api/register", &big)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	h.Register(w, r)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized body: got %d, want 413", w.Code)
	}

	// 2. JSON instead of multipart -> 400
	r = httptest.NewRequest(http.MethodPost, "/api/register", strings.NewReader(`{"email":"a@b.c"}`))
	r.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.Register(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("json body: got %d, want 400", w.Code)
	}

	// 3. Missing required field -> 400 with the field named, in our error shape
	var body bytes.Buffer
	mw = multipart.NewWriter(&body)
	mw.WriteField("email", "maria@example.com")
	mw.Close()
	r = httptest.NewRequest(http.MethodPost, "/api/register", &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	w = httptest.NewRecorder()
	h.Register(w, r)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `{"error":"password is required"}`) {
		t.Errorf("missing password: got %d %s", w.Code, w.Body.String())
	}
}

func validForm() url.Values {
	return url.Values{
		"email":         {"  Maria@Example.com "},
		"password":      {"secret123"},
		"first_name":    {" Γιάννης "},
		"last_name":     {"Papadopoulos"},
		"date_of_birth": {"2001-04-17"},
	}
}

func TestValidateRegisterValid(t *testing.T) {
	in, err := validateRegister(validForm())
	if err != nil {
		t.Fatal(err)
	}
	if in.Email != "maria@example.com" {
		t.Errorf("email not normalised: %q", in.Email)
	}
	if in.FirstName != "Γιάννης" {
		t.Errorf("first_name not trimmed: %q", in.FirstName)
	}
	if in.Nickname != nil || in.AboutMe != nil {
		t.Errorf("missing optional fields should be nil")
	}
}

func TestValidateRegisterOptionalFields(t *testing.T) {
	f := validForm()
	f.Set("nickname", "  mpap ")
	f.Set("about_me", "   ") // blank counts as not given
	in, err := validateRegister(f)
	if err != nil {
		t.Fatal(err)
	}
	if in.Nickname == nil || *in.Nickname != "mpap" {
		t.Errorf("nickname = %v", in.Nickname)
	}
	if in.AboutMe != nil {
		t.Errorf("blank about_me should be nil")
	}
}

func TestValidateRegisterErrors(t *testing.T) {
	cases := []struct {
		field, value, wantErr string
	}{
		{"email", "", "email is required"},
		{"email", "not-an-email", "email is invalid"},
		{"email", "Maria <maria@example.com>", "email is invalid"},
		{"password", "", "password is required"},
		{"password", "short", "password must be 8 to 72 characters"},
		{"password", strings.Repeat("a", 73), "password must be 8 to 72 characters"},
		{"first_name", "   ", "first_name is required"},
		{"last_name", strings.Repeat("x", 51), "last_name is too long"},
		{"date_of_birth", "", "date_of_birth is required"},
		{"date_of_birth", "17/04/2001", "date_of_birth must be a date in YYYY-MM-DD format"},
		{"date_of_birth", "2001-02-30", "date_of_birth must be a date in YYYY-MM-DD format"},
		{"date_of_birth", "2999-01-01", "date_of_birth must be a real date in the past"},
		{"nickname", strings.Repeat("n", 31), "nickname is too long"},
		{"about_me", strings.Repeat("a", 501), "about_me is too long"},
	}
	for _, c := range cases {
		f := validForm()
		f.Set(c.field, c.value)
		_, err := validateRegister(f)
		if err == nil || err.Error() != c.wantErr {
			t.Errorf("%s=%q: got %v, want %q", c.field, c.value, err, c.wantErr)
		}
	}
}

func TestValidateRegisterCountsCharactersNotBytes(t *testing.T) {
	f := validForm()
	f.Set("first_name", strings.Repeat("Ω", 50)) // 50 characters, 100 bytes
	if _, err := validateRegister(f); err != nil {
		t.Fatalf("50 Greek characters should be allowed: %v", err)
	}
}
