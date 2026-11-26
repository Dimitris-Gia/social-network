// Owner: A — register, login, logout, me
// Thin wrappers over B's request(): they only know the URLs from API.md 1.1.
// The session cookie is HttpOnly, so JS never sees it; request() sends it
// automatically with credentials: "include".
import { request } from "./client.js";

// formData: a FormData with the register fields (+ optional avatar file).
export const register = (formData) => request("/api/register", { method: "POST", body: formData });

export const login = (email, password) =>
  request("/api/login", { method: "POST", body: { email, password } });

export const logout = () => request("/api/logout", { method: "POST" });

// 200 -> the logged-in user, 401 -> throws (not logged in).
export const me = () => request("/api/me");