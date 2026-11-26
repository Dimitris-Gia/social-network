// Owner: A — file picker, accepts image/jpeg,png,gif, preview
//
// Reused by: RegisterPage (avatar), and later B's PostComposer / comments and
// C's group posts (image). It only *picks* a file; the parent puts it in a
// FormData and sends it.
//
// The type and size checks here are only for a nicer message. The real check
// is on the backend (utils.SaveImage reads the file's bytes with
// http.DetectContentType), because anyone can skip this page and call the API
// with curl.
import { useEffect, useRef, useState } from "react";

const ALLOWED_TYPES = ["image/jpeg", "image/png", "image/gif"];
const MAX_SIZE = 5 * 1024 * 1024; // 5 MB, same as utils.MaxImageSize

// Props:
//   label     text shown above the picker
//   name      the form field name the parent will use ("avatar" or "image")
//   file      the currently chosen File, or null (the parent owns the state)
//   onChange  called with a File or null
export default function ImageInput({ label = "Image", name = "image", file, onChange }) {
  const [error, setError] = useState("");
  const [previewUrl, setPreviewUrl] = useState(null);
  const inputRef = useRef(null);

  // Make a temporary blob: URL so <img> can show the file before it's uploaded.
  // The cleanup function frees it when the file changes or the component
  // unmounts, otherwise every new pick would leak memory.
  useEffect(() => {
    if (!file) {
      setPreviewUrl(null);
      return;
    }
    const url = URL.createObjectURL(file);
    setPreviewUrl(url);
    return () => URL.revokeObjectURL(url);
  }, [file]);

  function handlePick(e) {
    const picked = e.target.files?.[0] || null;
    setError("");
    if (!picked) {
      onChange(null);
      return;
    }
    if (!ALLOWED_TYPES.includes(picked.type)) {
      setError("Only JPEG, PNG or GIF images are allowed.");
      e.target.value = ""; // so picking the same bad file again still fires onChange
      onChange(null);
      return;
    }
    if (picked.size > MAX_SIZE) {
      setError("The image must be 5 MB or smaller.");
      e.target.value = "";
      onChange(null);
      return;
    }
    onChange(picked);
  }

  function handleRemove() {
    if (inputRef.current) inputRef.current.value = "";
    setError("");
    onChange(null);
  }

  return (
    <div className="image-input">
      <label>
        {label}
        <input ref={inputRef} type="file" name={name}
          accept="image/jpeg,image/png,image/gif" onChange={handlePick} />
      </label>
      {previewUrl && (
        <div className="image-input-preview">
          <img src={previewUrl} alt="Selected preview" />
          <button type="button" onClick={handleRemove}>Remove</button>
        </div>
      )}
      {error && <p className="form-error" role="alert">{error}</p>}
    </div>
  );
}

// Owner: A — user avatar
// Takes any object with the UserSummary shape (API.md Part 0). Shows the
// uploaded picture, or the user's initials when there is none.
//
// Safe from XSS: `avatar` is a path the SERVER built ("/uploads/<uuid>.png"),
// never something the user typed, and React escapes the initials as text.
export default function Avatar({ user, size = 40 }) {
  const style = { width: size, height: size };

  if (user?.avatar) {
    return <img className="avatar" src={user.avatar} alt="" style={style} />;
  }

  const initials = `${user?.first_name?.[0] ?? ""}${user?.last_name?.[0] ?? ""}`.toUpperCase();
  return (
    <span className="avatar avatar-initials" style={{ ...style, fontSize: size * 0.4 }} aria-hidden="true">
      {initials || "?"}
    </span>
  );
}

// The display rule from API.md: nickname if set, otherwise "First Last".
// Exported so every page names people the same way.
export function displayName(user) {
  return user?.nickname || `${user?.first_name ?? ""} ${user?.last_name ?? ""}`.trim();
}