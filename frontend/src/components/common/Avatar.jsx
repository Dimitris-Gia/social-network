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