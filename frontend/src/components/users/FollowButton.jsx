// Owner: A — Follow / Requested / Unfollow
//
// A "controlled" button: the PARENT owns follow_status (it came from the
// server in GET /api/users or GET /api/users/{id}) and passes it in. After a
// click we tell the parent the new status with onChange(newStatus), and the
// parent decides what else to refresh (the profile page reloads the whole
// profile, the users list just updates one row).
//
// The button never guesses the new status itself after a follow: the server
// answers "following" for a public profile and "pending" for a private one,
// and we use that answer. Only the server knows whether the profile is
// private right now (the owner may have just flipped the toggle).
//
//   status    | label      | click does
//   "none"      Follow       POST   /api/users/{id}/follow
//   "pending"   Requested    DELETE /api/users/{id}/follow  (cancel request)
//   "following" Unfollow     DELETE /api/users/{id}/follow
//   "self"      (nothing: you can't follow yourself)
import { useState } from "react";
import * as usersApi from "../../api/users.js";

const LABELS = {
  none: "Follow",
  pending: "Requested",
  following: "Unfollow",
};

// What a screen reader / hover tooltip says the click will do. "Requested"
// alone doesn't tell you that clicking it cancels the request.
const HINTS = {
  none: "Follow",
  pending: "Cancel follow request",
  following: "Unfollow",
};

export default function FollowButton({ userId, status, onChange }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  if (status === "self" || !LABELS[status]) return null;

  async function handleClick() {
    setBusy(true);
    setError("");
    try {
      if (status === "none") {
        const res = await usersApi.follow(userId);
        onChange?.(res.follow_status); // "following" or "pending"
      } else {
        // Bonus (Fri 4): ask with <ConfirmDialog> before unfollowing.
        await usersApi.unfollow(userId);
        onChange?.("none");
      }
    } catch (err) {
      // Usually a stale button: e.g. in another browser tab you already
      // followed them (409) or they declined your request (404). Show the
      // server's message; the parent's next reload fixes the label.
      setError(err.message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <span className="follow-button-wrap">
      <button
        type="button"
        className={`follow-button follow-button-${status}`}
        onClick={handleClick}
        disabled={busy} // no double POST from a double click
        title={HINTS[status]}
        aria-label={HINTS[status]}
      >
        {busy ? "…" : LABELS[status]}
      </button>
      {error && <span className="form-error follow-button-error">{error}</span>}
    </span>
  );
}