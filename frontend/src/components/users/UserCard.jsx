// Owner: A — user summary card
// One row for a user: avatar + name, linking to their profile. Used in the
// followers/following tabs today and in UsersPage tomorrow. `children` is a
// slot on the right, e.g. a FollowButton.
import { Link } from "react-router-dom";
import Avatar, { displayName } from "../common/Avatar.jsx";

export default function UserCard({ user, children }) {
  return (
    <div className="user-card">
      <Link to={`/users/${user.id}`} className="user-card-link">
        <Avatar user={user} size={36} />
        <span>{displayName(user)}</span>
      </Link>
      {children}
    </div>
  );
}