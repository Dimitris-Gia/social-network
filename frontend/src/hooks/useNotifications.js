// Owner: A — list + unread count, live updates from WS
//
// All the notification STATE lives here; NotificationBell only draws it.
// Keeping it in a hook means the bell component stays about the UI, and the
// rules ("when do we refetch?", "what does a live push do?") are in one place.
//
// Where the numbers come from:
//   - badge count: GET /api/notifications/unread-count on load, then +1 for
//     every live push, 0 when the dropdown opens (read-all).
//   - list: GET /api/notifications every time the dropdown opens, so it is
//     never stale (e.g. a requester cancelled their follow request while we
//     weren't looking: the server already deleted that notification).
//   - live: the server's notify() pushes {"type":"notification","data":{...}}
//     through hub.SendToUser. Only the receiver's tabs get it. We listen with
//     B's useSocketEvent, on the app's single WebSocket (WebSocketProvider).
//   - after a dropped connection: B's socket manager fires "ws:reconnected".
//     Pushes sent while we were offline are lost (they're only in the DB),
//     so we re-fetch the count then.
import { useCallback, useEffect, useRef, useState } from "react";
import * as notificationsApi from "../api/notifications.js";
import useSocketEvent from "./useSocketEvent.js";

export default function useNotifications() {
  const [notifications, setNotifications] = useState([]);
  const [unreadCount, setUnreadCount] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  // The live handler needs to know whether the dropdown is open. A ref, so
  // opening/closing doesn't re-render anything just for the handler.
  const openRef = useRef(false);

  const refreshCount = useCallback(async () => {
    try {
      const { count } = await notificationsApi.unreadNotificationCount();
      setUnreadCount(count);
    } catch {
      // The badge is not worth an error message. 401 means logged out,
      // and then the Navbar stops rendering the bell anyway.
    }
  }, []);

  // Badge on first render (the bell mounts right after login).
  useEffect(() => {
    refreshCount();
  }, [refreshCount]);

  // Back online after a drop: catch up on what we missed.
  useSocketEvent("ws:reconnected", refreshCount);

  // Live pushes.
  useSocketEvent("notification", (n) => {
    if (!n || typeof n.id !== "number") return; // never trust a frame's shape
    setNotifications((list) => (list.some((x) => x.id === n.id) ? list : [n, ...list]));
    if (openRef.current) {
      // The user is looking at the list right now, so it's read.
      notificationsApi.markNotificationRead(n.id).catch(() => {});
    } else {
      setUnreadCount((c) => c + 1);
    }
  });

  // Called by the bell when the dropdown opens: fresh list, then mark all as
  // read on the server (API.md 1.4). We keep the is_read values from the
  // fetch in the list, so the ones that were new stay highlighted while the
  // dropdown is open.
  const open = useCallback(async () => {
    openRef.current = true;
    setLoading(true);
    setError("");
    try {
      const list = await notificationsApi.listNotifications();
      setNotifications(list);
      if (list.some((n) => !n.is_read)) {
        await notificationsApi.markAllNotificationsRead();
      }
      setUnreadCount(0);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }, []);

  // On close, everything shown has been seen: drop the highlight.
  const close = useCallback(() => {
    openRef.current = false;
    setNotifications((list) => list.map((n) => (n.is_read ? n : { ...n, is_read: true })));
  }, []);

  // After Accept / Decline the server deleted the notification, so we remove
  // it locally too instead of refetching the whole list.
  const remove = useCallback((id) => {
    setNotifications((list) => list.filter((n) => n.id !== id));
  }, []);

  return { notifications, unreadCount, loading, error, open, close, remove };
}