// Owner: B — links, NotificationBell, ChatIcon, logout (A wires it)
import { Link } from "react-router-dom";

export default function Navbar() {
  return (
    <nav>
      <Link to="/">Feed</Link> | <Link to="/users">People</Link> |{" "}
      <Link to="/groups">Groups</Link> | <Link to="/chat">Chat</Link>
      {/* TODO: <NotificationBell /> <ChatIcon /> logout */}
    </nav>
  );
}
