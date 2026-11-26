// Owner: A — reads AuthContext
// Usage in any component: const { user, loading, login, logout } = useAuth();
import { useContext } from "react";
import { AuthContext } from "../context/AuthContext.jsx";

export default function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) {
    // Catches the mistake early instead of "cannot read user of null" later.
    throw new Error("useAuth must be used inside <AuthProvider>");
  }
  return ctx;
}