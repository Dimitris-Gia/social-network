// Owner: A — badge + dropdown (on every page via Navbar)
//
// The Navbar is rendered once in App.jsx, outside <Routes>, so this bell is
// on every page (audit: "notifications visible on every page"). It is a
// separate component from B's ChatIcon, with its own badge and its own API
// (/api/notifications/*), so new chat messages never show up here
// (audit: notifications kept separate from private messages).
//
// Opening the dropdown: fetch the list, mark all read, badge -> 0.
// Closing it: click the bell again, click anywhere outside, press Escape,
// or follow a link inside it.
import { useEffect, useRef, useState } from "react";
import useNotifications from "../../hooks/useNotifications.js";
import NotificationItem from "./NotificationItem.jsx";

export default function NotificationBell() {
  const { notifications, unreadCount, loading, error, open, close, remove } = useNotifications();
  const [isOpen, setIsOpen] = useState(false);
  const wrapRef = useRef(null);

  function toggle() {
    if (isOpen) {
      setIsOpen(false);
      close();
    } else {
      setIsOpen(true);
      open();
    }
  }

  // Close on a click outside the bell or on Escape. Listeners exist only
  // while the dropdown is open.
  useEffect(() => {
    if (!isOpen) return undefined;

    function onPointerDown(e) {
      if (wrapRef.current && !wrapRef.current.contains(e.target)) {
        setIsOpen(false);
        close();
      }
    }
    function onKeyDown(e) {
      if (e.key === "Escape") {
        setIsOpen(false);
        close();
      }
    }
    document.addEventListener("mousedown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("mousedown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [isOpen, close]);

  function handleNavigate() {
    setIsOpen(false);
    close();
  }

  const badge = unreadCount > 99 ? "99+" : String(unreadCount);
  const label = unreadCount > 0 ? `Notifications, ${unreadCount} unread` : "Notifications";

  return (
    <div className="notification-bell" ref={wrapRef}>
      <button
        type="button"
        className="notification-bell-button"
        onClick={toggle}
        aria-label={label}
        aria-haspopup="true"
        aria-expanded={isOpen}
        title="Notifications"
      >
        <span aria-hidden="true">🔔</span>
        {unreadCount > 0 && (
          <span className="notification-badge" aria-hidden="true">
            {badge}
          </span>
        )}
      </button>

      {isOpen && (
        <div className="notification-dropdown" role="region" aria-label="Notifications">
          <h2 className="notification-dropdown-title">Notifications</h2>
          {error && <p className="form-error notification-empty">{error}</p>}
          {!error && loading && notifications.length === 0 && (
            <p className="notification-empty">Loading…</p>
          )}
          {!error && !loading && notifications.length === 0 && (
            <p className="notification-empty">No notifications yet.</p>
          )}
          {notifications.length > 0 && (
            <ul className="notification-list">
              {notifications.map((n) => (
                <NotificationItem
                  key={n.id}
                  notification={n}
                  onResolved={remove}
                  onNavigate={handleNavigate}
                />
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}