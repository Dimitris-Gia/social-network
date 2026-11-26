// Owner: B — one WS per logged-in user, subscribe(type, handler)
//
// The provider opens the connection when somebody is logged in and closes it
// when they log out (or another user logs in). Everything else in the app
// talks to it through the hooks:
//
//   const { send, status } = useWebSocket();          // hooks/useWebSocket.js
//   useSocketEvent("notification", (n) => ...);       // hooks/useSocketEvent.js
//
// The connection logic itself (reconnect, parsing, subscribers) lives in
// socketManager.js and is tested there.
import { createContext, useEffect, useMemo, useRef, useState } from "react";
import useAuth from "../hooks/useAuth.js";
import * as authApi from "../api/auth.js";
import { createSocketManager } from "./socketManager.js";

export const WebSocketContext = createContext(null);

// Must sit INSIDE <AuthProvider> (it reads the logged-in user from it).
export function WebSocketProvider({ children }) {
  const { user } = useAuth();
  // Depend on the ID, not on the user object: A's setUser() creates a new
  // object when the profile changes, and that must not reconnect the socket.
  const userId = user?.id ?? null;

  // One manager for the lifetime of the provider. It does nothing until connect().
  const managerRef = useRef(null);
  if (managerRef.current === null) {
    managerRef.current = createSocketManager({
      // A failed (re)connect may mean the session expired: the browser can't
      // show us the 401 from /ws, so ask /api/me. A 401 there goes through
      // client.js's handler -> AuthContext forgets the user -> this provider
      // disconnects (no more retries) and the user lands on /login.
      // Any other failure (server down, network) is ignored: we keep retrying.
      onRetry: () => {
        authApi.me().catch(() => {});
      },
    });
  }
  const manager = managerRef.current;

  const [status, setStatus] = useState(manager.getStatus());
  useEffect(() => {
    setStatus(manager.getStatus());
    return manager.onStatusChange(setStatus);
  }, [manager]);

  useEffect(() => {
    if (userId === null) return undefined; // not logged in (or /api/me still loading)
    manager.connect();
    // Runs on logout and when a different user logs in. (In development,
    // StrictMode also runs it once right after mounting; connect() and
    // disconnect() are built to survive that.)
    return () => manager.disconnect();
  }, [userId, manager]);

  // subscribe and send never change, so consumers do not re-render because of them.
  const value = useMemo(
    () => ({ subscribe: manager.subscribe, send: manager.send, status }),
    [manager, status]
  );

  return <WebSocketContext.Provider value={value}>{children}</WebSocketContext.Provider>;
}