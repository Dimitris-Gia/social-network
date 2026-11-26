// Owner: A — inline Accept / Decline
//
// One row in the bell's dropdown. What it shows and which buttons it has
// depend only on `type` (API.md 1.4). There is no "accept notification"
// endpoint: the buttons call the real domain endpoint (here
// POST /api/follow-requests/{actor.id}/accept), and THAT endpoint deletes the
// notification on the server. We then remove it from the list with onResolved.
//
// Why actor.id is safe to send: the server's query is
//   ... WHERE follower_id = <actor.id> AND following_id = <logged-in user>
// so even a hand-edited request can only answer requests addressed to YOU.
//
// XSS: names and titles are rendered as text children, so React escapes
// them. A group called "<img onerror=...>" shows up literally, never runs.
import { useState } from "react";
import { Link } from "react-router-dom";
import Avatar, { displayName } from "../common/Avatar.jsx";
import * as notificationsApi from "../../api/notifications.js";

// "5m ago" style. Plain JS, no date library needed.
function timeAgo(iso) {
  const seconds = Math.max(0, Math.floor((Date.now() - new Date(iso).getTime()) / 1000));
  if (seconds < 60) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 7) return `${days}d ago`;
  return new Date(iso).toLocaleDateString();
}

export default function NotificationItem({ notification, onResolved, onNavigate }) {
  const { id, type, is_read: isRead, created_at: createdAt, actor, group, event } = notification;
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  // Links close the dropdown (onNavigate) so the new page isn't covered.
  const actorLink = actor ? (
    <Link to={`/users/${actor.id}`} onClick={onNavigate} className="notification-strong">
      {displayName(actor)}
    </Link>
  ) : (
    <span className="notification-strong">Someone</span>
  );
  const groupLink = group ? (
    <Link to={`/groups/${group.id}`} onClick={onNavigate} className="notification-strong">
      {group.title}
    </Link>
  ) : null;

  let text;
  let actions = null;

  switch (type) {
    case "follow_request":
      text = <>{actorLink} wants to follow you</>;
      actions = { accept: notificationsApi.acceptFollowRequest, decline: notificationsApi.declineFollowRequest };
      break;
    case "group_invite":
      // Accept / Decline buttons for groups come on Fri 4 (paired with C).
      text = <>{actorLink} invited you to join {groupLink}</>;
      break;
    case "group_join_request":
      text = <>{actorLink} wants to join {groupLink}</>;
      break;
    case "group_event":
      text = (
        <>
          {actorLink} created the event <span className="notification-strong">{event?.title}</span> in {groupLink}
        </>
      );
      break;
    case "follow_accepted":
      text = <>{actorLink} accepted your follow request</>;
      break;
    case "new_follower":
      text = <>{actorLink} started following you</>;
      break;
    default:
      // An unknown type from a newer backend: show something, don't crash.
      text = <>New notification</>;
  }

  async function answer(call) {
    if (!actor) return;
    setBusy(true);
    setError("");
    try {
      await call(actor.id);
      onResolved(id); // server already deleted it; drop it locally
    } catch (err) {
      // Usually 404: the requester cancelled in the meantime. Show why;
      // the list is refetched the next time the bell opens.
      setError(err.message);
      setBusy(false);
    }
  }

  return (
    <li className={`notification-item${isRead ? "" : " notification-unread"}`}>
      {actor && <Avatar user={actor} size={36} />}
      <div className="notification-body">
        <p className="notification-text">{text}</p>
        <time className="notification-time" dateTime={createdAt}>
          {timeAgo(createdAt)}
        </time>
        {actions && (
          <div className="notification-actions">
            <button type="button" className="btn-accept" disabled={busy} onClick={() => answer(actions.accept)}>
              Accept
            </button>
            <button type="button" className="btn-decline" disabled={busy} onClick={() => answer(actions.decline)}>
              Decline
            </button>
          </div>
        )}
        {error && <p className="form-error notification-error">{error}</p>}
      </div>
    </li>
  );
}