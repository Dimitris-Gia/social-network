package handlers

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

	"social-network/pkg/middleware"
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

// ── Login / logout / me (Wed 25) ────────────────────────────────────────────

// loginRequest is the JSON body of POST /api/login. ReadJSON rejects unknown
// fields, so {"emial": ...} is a clear 400 instead of "email is required".
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// errBadLogin is the ONE message for "no such email" and "wrong password".
// If they were different, anyone could use the login form to find out which
// emails have an account.
const errBadLogin = "invalid email or password"

// dummyHash is compared against when the email doesn't exist, so a login with
// an unknown email takes as long as one with a wrong password (~50-100 ms of
// bcrypt). Without it, the fast answer would give the email away even though
// the message is the same.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("not-a-real-password"), bcrypt.DefaultCost)

// Login handles POST /api/login (public).
//
//	curl -i -c cookies.txt -X POST localhost:8080/api/login \
//	  -H 'Content-Type: application/json' \
//	  -d '{"email":"maria@example.com","password":"secret123"}'
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := utils.ReadJSON(w, r, &req); err != nil {
		utils.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Same normalisation as Register, otherwise "Maria@x.com" could never log in.
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" || req.Password == "" {
		utils.WriteError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	user, err := models.GetUserByEmail(h.DB, email)
	if errors.Is(err, sql.ErrNoRows) {
		bcrypt.CompareHashAndPassword(dummyHash, []byte(req.Password)) // equal timing, result ignored
		utils.WriteError(w, http.StatusUnauthorized, errBadLogin)
		return
	}
	if err != nil {
		utils.ServerError(w, err)
		return
	}

	// bcrypt re-hashes the typed password with the salt stored inside the hash
	// and compares the results in constant time. We never decrypt anything:
	// bcrypt is one-way.
	if len(req.Password) > 72 ||
		bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		utils.WriteError(w, http.StatusUnauthorized, errBadLogin)
		return
	}

	// If this browser already had a session (e.g. user A logs in as user B
	// without logging out), delete the old row. Every login gets a NEW random
	// ID; we never reuse an ID the browser sent us (prevents session fixation).
	if old, err := r.Cookie(middleware.CookieName); err == nil {
		if err := models.DeleteSession(h.DB, old.Value); err != nil {
			log.Printf("login: delete old session: %v", err)
		}
	}
	// Housekeeping only, so a failure here must not block the login.
	if err := models.DeleteExpiredSessions(h.DB, user.ID); err != nil {
		log.Printf("login: delete expired sessions: %v", err)
	}

	sessionID, err := models.CreateSession(h.DB, user.ID)
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	middleware.SetSessionCookie(w, sessionID)

	user.FollowStatus = "self"
	user.CanView = true
	utils.WriteJSON(w, http.StatusOK, user)
}

// Logout handles POST /api/logout (protected).
//
// Deleting the row is what really logs you out: even if someone copied the
// cookie, the ID no longer exists in the DB, so RequireAuth answers 401.
// Clearing the cookie just tidies up the browser.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	// RequireAuth already checked this cookie exists and is valid.
	cookie, err := r.Cookie(middleware.CookieName)
	if err == nil {
		if err := models.DeleteSession(h.DB, cookie.Value); err != nil {
			utils.ServerError(w, err)
			return
		}
	}
	middleware.ClearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// Me handles GET /api/me (protected). The React AuthContext calls it on every
// page load: 200 = logged in (and here is who), 401 = show the login page.
// The frontend can't read the HttpOnly cookie, so this is how it finds out.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserID(r)
	user, err := models.GetUserByID(h.DB, userID)
	if errors.Is(err, sql.ErrNoRows) {
		// Can only happen if the user row was deleted (sessions cascade with it).
		middleware.ClearSessionCookie(w)
		utils.WriteError(w, http.StatusUnauthorized, "not logged in")
		return
	}
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	// TODO(A, Thu 26): fill FollowersCount / FollowingCount once follow.go exists.
	user.FollowStatus = "self"
	user.CanView = true
	utils.WriteJSON(w, http.StatusOK, user)
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
