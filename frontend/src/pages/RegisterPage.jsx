// Owner: A — 8 fields, 3 optional
//
// Required (assignment + audit): email, password, first name, last name,
// date of birth. Optional but always visible: avatar, nickname, about me.
//
// Flow: POST /api/register (multipart, because of the avatar) -> 201, then
// POST /api/login with the same email + password (API.md 1.1: register does
// not log in) -> AuthContext now has the user -> go to the feed.
//
// The checks here (required, lengths, date in the past) only save a round
// trip. The backend's validateRegister() repeats every one of them, because
// the API can be called without this page.
import { useState } from "react";
import { Link, Navigate, useNavigate } from "react-router-dom";
import useAuth from "../hooks/useAuth.js";
import { register } from "../api/auth.js";
import ImageInput from "../components/common/ImageInput.jsx";

// Today as YYYY-MM-DD in the user's local time, used as the date picker's max.
// (toISOString() would give the UTC date, which is "tomorrow" late at night.)
function todayLocal() {
  const d = new Date();
  const mm = String(d.getMonth() + 1).padStart(2, "0");
  const dd = String(d.getDate()).padStart(2, "0");
  return `${d.getFullYear()}-${mm}-${dd}`;
}

const EMPTY_FORM = {
  email: "",
  password: "",
  first_name: "",
  last_name: "",
  date_of_birth: "",
  nickname: "",
  about_me: "",
};

export default function RegisterPage() {
  const { user, loading, login } = useAuth();
  const navigate = useNavigate();

  // One state object for all text fields; keys are the exact API field names,
  // so building the FormData is a simple loop.
  const [form, setForm] = useState(EMPTY_FORM);
  const [avatar, setAvatar] = useState(null); // File or null
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  if (loading) return null;
  if (user) return <Navigate to="/" replace />; // already logged in

  // One handler for every input: the input's name says which key to update.
  function handleChange(e) {
    const { name, value } = e.target;
    setForm((f) => ({ ...f, [name]: value }));
  }

  async function handleSubmit(e) {
    e.preventDefault();
    setError("");

    if (form.date_of_birth && form.date_of_birth >= todayLocal()) {
      setError("Date of birth must be in the past.");
      return;
    }

    // FormData = multipart/form-data. Don't set Content-Type yourself: the
    // browser adds it with the boundary string. client.js already skips the
    // JSON header when the body is FormData.
    const data = new FormData();
    for (const [key, value] of Object.entries(form)) {
      // Password is sent exactly as typed (spaces can be part of it);
      // everything else is trimmed. Empty optional fields become null on the
      // backend.
      data.append(key, key === "password" ? value : value.trim());
    }
    if (avatar) data.append("avatar", avatar);

    setSubmitting(true);
    try {
      await register(data);
    } catch (err) {
      // e.g. 409 "email already registered", 400 "first_name is required",
      // 415 for a bad avatar. Rendered as text, so React escapes it (no XSS).
      setError(err.message);
      setSubmitting(false);
      return;
    }

    try {
      await login(form.email.trim(), form.password);
      navigate("/", { replace: true });
    } catch {
      // The account exists but auto-login failed (e.g. server restarted).
      // Send them to the login page instead of leaving them stuck here.
      navigate("/login", { replace: true });
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="auth-page">
      <h1>Create an account</h1>
      <form onSubmit={handleSubmit} className="auth-form">
        <label>
          Email *
          <input type="email" name="email" value={form.email} onChange={handleChange}
            maxLength={100} autoComplete="email" required />
        </label>
        <label>
          Password * <small>(8–72 characters)</small>
          <input type="password" name="password" value={form.password} onChange={handleChange}
            minLength={8} maxLength={72} autoComplete="new-password" required />
        </label>
        <label>
          First name *
          <input type="text" name="first_name" value={form.first_name} onChange={handleChange}
            maxLength={50} autoComplete="given-name" required />
        </label>
        <label>
          Last name *
          <input type="text" name="last_name" value={form.last_name} onChange={handleChange}
            maxLength={50} autoComplete="family-name" required />
        </label>
        <label>
          Date of birth *
          <input type="date" name="date_of_birth" value={form.date_of_birth} onChange={handleChange}
            min="1900-01-01" max={todayLocal()} required />
        </label>

        <fieldset className="optional-fields">
          <legend>Optional</legend>
          <ImageInput label="Avatar" name="avatar" file={avatar} onChange={setAvatar} />
          <label>
            Nickname
            <input type="text" name="nickname" value={form.nickname} onChange={handleChange}
              maxLength={30} autoComplete="nickname" />
          </label>
          <label>
            About me
            <textarea name="about_me" value={form.about_me} onChange={handleChange}
              maxLength={500} rows={4} />
          </label>
        </fieldset>

        {error && <p className="form-error" role="alert">{error}</p>}
        <button type="submit" disabled={submitting}>
          {submitting ? "Creating account…" : "Register"}
        </button>
      </form>
      <p>
        Already have an account? <Link to="/login">Log in</Link>
      </p>
    </div>
  );
}