// Owner: A — redirect to /login if not logged in
//
// Used as a *layout route* in App.jsx: every <Route> nested inside it is
// rendered through <Outlet /> only when someone is logged in.
//
// This is a UX guard, not the security boundary. Hiding a page in React
// doesn't protect data: a user can call the API directly. The real protection
// is middleware.RequireAuth on the backend, which answers 401 to every
// protected /api route without a valid session cookie.
import { Navigate, Outlet, useLocation } from "react-router-dom";
import useAuth from "../../hooks/useAuth.js";

export default function ProtectedRoute() {
  const { user, loading } = useAuth();
  const location = useLocation();

  // While /api/me is still answering we don't know yet. Redirecting now would
  // bounce a logged-in user to /login on every refresh.
  if (loading) return <p className="page-loading">Loading…</p>;

  if (!user) {
    // Remember where they wanted to go; LoginPage reads state.from and sends
    // them back there after logging in. `replace` keeps the protected URL out
    // of the history, so Back doesn't loop them into the redirect again.
    return <Navigate to="/login" replace state={{ from: location }} />;
  }

  return <Outlet />;
}