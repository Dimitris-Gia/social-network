package handlers

import (
	"errors"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

	"social-network/pkg/models"
	"social-network/pkg/utils"
)

// registerInput holds the register form after it has been cleaned and checked.
type registerInput struct {
	Email       string
	Password    string
	FirstName   string
	LastName    string
	DateOfBirth string
	Nickname    *string
	AboutMe     *string
}

// Register handles POST /api/register (public).
//
// Order matters: parse -> validate text -> check email -> hash -> save image
// -> insert. Cheap checks come first, and the image is written to disk only
// once we're almost sure the account will be created, so bad requests don't
// leave stray files behind.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	// Cap the whole body before reading any of it, so nobody can send 2 GB.
	r.Body = http.MaxBytesReader(w, r.Body, utils.MaxUploadBody)
	if err := r.ParseMultipartForm(utils.MultipartMemory); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			utils.WriteError(w, http.StatusRequestEntityTooLarge, "request too large")
			return
		}
		utils.WriteError(w, http.StatusBadRequest, "expected multipart/form-data")
		return
	}
	defer r.MultipartForm.RemoveAll() // delete Go's temp files for big uploads

	// r.PostForm holds only body fields (r.Form would also include ?query=
	// values from the URL, which we don't want to accept here).
	in, err := validateRegister(r.PostForm)
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	exists, err := models.EmailExists(h.DB, in.Email)
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	if exists {
		utils.WriteError(w, http.StatusConflict, models.ErrEmailTaken.Error())
		return
	}

	// bcrypt hashes with a random salt and is deliberately slow, so a leaked
	// DB can't be reversed quickly and two equal passwords get different hashes.
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		utils.ServerError(w, err)
		return
	}

	avatar, status, err := utils.SaveImage(r, "avatar")
	if err != nil {
		utils.WriteError(w, status, err.Error())
		return
	}

	user := &models.User{
		Email:        in.Email,
		PasswordHash: string(hash),
		FirstName:    in.FirstName,
		LastName:     in.LastName,
		DateOfBirth:  in.DateOfBirth,
		Nickname:     in.Nickname,
		AboutMe:      in.AboutMe,
		Avatar:       avatar,
	}
	if err := models.CreateUser(h.DB, user); err != nil {
		utils.DeleteImage(avatar) // the account wasn't created, so drop its avatar
		if errors.Is(err, models.ErrEmailTaken) {
			utils.WriteError(w, http.StatusConflict, err.Error())
			return
		}
		utils.ServerError(w, err)
		return
	}

	user.FollowStatus = "self"
	user.CanView = true
	utils.WriteJSON(w, http.StatusCreated, user)
}

// validateRegister applies the rules in API.md 1.1. It takes url.Values (not
// the request) so it can be unit-tested without building multipart bodies.
func validateRegister(form url.Values) (registerInput, error) {
	var in registerInput

	// Lowercase so "Maria@x.com" and "maria@x.com" are the same account;
	// otherwise the duplicate-email check could be bypassed with capitals.
	in.Email = strings.ToLower(strings.TrimSpace(form.Get("email")))
	if in.Email == "" {
		return in, errors.New("email is required")
	}
	// ParseAddress also accepts "Name <a@b.c>", so require that the parsed
	// address is exactly what was typed.
	if addr, err := mail.ParseAddress(in.Email); err != nil || addr.Address != in.Email || len(in.Email) > 100 {
		return in, errors.New("email is invalid")
	}

	// Not trimmed: spaces can be part of a password. bcrypt only reads the
	// first 72 bytes, so longer passwords are refused instead of silently cut.
	in.Password = form.Get("password")
	if in.Password == "" {
		return in, errors.New("password is required")
	}
	if len(in.Password) < 8 || len(in.Password) > 72 {
		return in, errors.New("password must be 8 to 72 characters")
	}

	var err error
	if in.FirstName, err = requiredText(form, "first_name", 50); err != nil {
		return in, err
	}
	if in.LastName, err = requiredText(form, "last_name", 50); err != nil {
		return in, err
	}

	in.DateOfBirth = strings.TrimSpace(form.Get("date_of_birth"))
	if in.DateOfBirth == "" {
		return in, errors.New("date_of_birth is required")
	}
	dob, err := time.Parse("2006-01-02", in.DateOfBirth) // also rejects 2001-02-30
	if err != nil {
		return in, errors.New("date_of_birth must be a date in YYYY-MM-DD format")
	}
	if !dob.Before(time.Now()) || dob.Year() < 1900 {
		return in, errors.New("date_of_birth must be a real date in the past")
	}

	if in.Nickname, err = optionalText(form, "nickname", 30); err != nil {
		return in, err
	}
	if in.AboutMe, err = optionalText(form, "about_me", 500); err != nil {
		return in, err
	}
	return in, nil
}

// requiredText trims a field and checks it's 1..max characters. Characters
// are counted with utf8.RuneCountInString, so a Greek name like "Γιάννης"
// counts as 7, not 14 (its byte length).
func requiredText(form url.Values, field string, max int) (string, error) {
	v := strings.TrimSpace(form.Get(field))
	if v == "" {
		return "", errors.New(field + " is required")
	}
	if utf8.RuneCountInString(v) > max {
		return "", errors.New(field + " is too long")
	}
	return v, nil
}

// optionalText returns nil for a missing or blank field, so the DB stores NULL
// and the JSON says null, instead of an empty string.
func optionalText(form url.Values, field string, max int) (*string, error) {
	v := strings.TrimSpace(form.Get(field))
	if v == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(v) > max {
		return nil, errors.New(field + " is too long")
	}
	return &v, nil
}
