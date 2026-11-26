// Owner: A — current user from GET /api/me, login(), logout()
//
// One place in the app knows who is logged in. On the first load it asks the
// backend (GET /api/me) because the cookie is HttpOnly: JS can't read it, so
// the server is the only one who can say whether the session is valid.
// After a refresh the cookie is still there, so /api/me returns the same
// user again: that's how "stay logged in after refresh" works (audit).
//
// Session expired while the page is open: any API call then gets a 401.
// client.js reports it here and we forget the user. That one change does
// the rest: WebSocketProvider closes the socket (no more retries) and
// ProtectedRoute redirects to /login, remembering the page they were on.
import { createContext, useCallback, useEffect, useMemo, useState } from "react";
import * as authApi from "../api/auth.js";
import { setUnauthorizedHandler } from "../api/client.js";

export const AuthContext = createContext(null);

export function AuthProvider({ children }) {
  const [user, setUser] = useState(null);
  // true until /api/me has answered, so ProtectedRoute doesn't bounce a
  // logged-in user to /login during the first split second.
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    setUnauthorizedHandler(() => setUser(null));
    return () => setUnauthorizedHandler(() => {});
  }, []);

  useEffect(() => {
    authApi
      .me()
      .then(setUser)
      .catch(() => setUser(null)) // 401 = not logged in
      .finally(() => setLoading(false));
  }, []);

  const login = useCallback(async (email, password) => {
    const u = await authApi.login(email, password); // throws with the server's message
    setUser(u);
    return u;
  }, []);

  const logout = useCallback(async () => {
    try {
      await authApi.logout();
    } finally {
      setUser(null); // even if the request failed, forget the user locally
    }
  }, []);

  // useMemo: consumers only re-render when one of these really changes.
  const value = useMemo(
    () => ({ user, setUser, loading, login, logout }),
    [user, loading, login, logout]
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}