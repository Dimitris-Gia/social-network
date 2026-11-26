// Owner: B — links, NotificationBell, ChatIcon, logout (A wires it)
//
// Rendered once in App.jsx, outside <Routes>, so it is on every page. That's
// why the bell and the logout button live here (audit: notifications visible
// on every page; logout reachable from anywhere).
import { Link, NavLink, useNavigate } from "react-router-dom";
import useAuth from "../../hooks/useAuth.js";

export default function Navbar() {
  const { user, loading, logout } = useAuth();
  const navigate = useNavigate();

  async function handleLogout() {
    // logout() calls POST /api/logout, which deletes the session row and
    // expires the cookie, then clears the user in AuthContext even if the
    // request failed. Then we leave the protected page.
    await logout();
    navigate("/login", { replace: true });
  }

  // Until /api/me answers, show only the brand so links don't flicker.
  if (loading) {
    return (
      <nav className="navbar">
        <span className="navbar-brand">Social Network</span>
      </nav>
    );
  }

  if (!user) {
    return (
      <nav className="navbar">
        <span className="navbar-brand">Social Network</span>
        <div className="navbar-right">
          <NavLink to="/login">Log in</NavLink>
          <NavLink to="/register">Register</NavLink>
        </div>
      </nav>
    );
  }

  // Display rule from API.md: nickname if set, else "First Last".
  // Rendered as text, so React escapes it (a nickname like <script> is harmless).
  const displayName = user.nickname || `${user.first_name} ${user.last_name}`;

  return (
    <nav className="navbar">
      <Link to="/" className="navbar-brand">Social Network</Link>
      <div className="navbar-links">
        <NavLink to="/" end>Feed</NavLink>
        <NavLink to="/users">People</NavLink>
        <NavLink to="/groups">Groups</NavLink>
        <NavLink to="/chat">Chat</NavLink>
      </div>
      <div className="navbar-right">
        {/* TODO (A, Thu): <NotificationBell />   TODO (B, Fri): <ChatIcon /> */}
        <NavLink to={`/users/${user.id}`}>{displayName}</NavLink>
        <button type="button" onClick={handleLogout}>Log out</button>
      </div>
    </nav>
  );
}