// Owner: A — profiles, follows
// Thin wrappers over B's request(): they only know the URLs from API.md 1.2.
// request() throws an Error whose message is the server's {"error": "..."},
// so a page can show err.message directly.
import { request } from "./client.js";

// Everyone except you, for the "find people" page (Wed).
export const listUsers = () => request("/api/users");

// Always 200 for an existing user. If can_view is false the server sends only
// the summary fields: the rest of the profile never reaches the browser.
export const getUser = (id) => request(`/api/users/${id}`);

// Gated on the server: 403 "this account is private" for a non-follower.
export const getFollowers = (id) => request(`/api/users/${id}/followers`);
export const getFollowing = (id) => request(`/api/users/${id}/following`);

// Owned by B on the backend, but it lives under /api/users, so the wrapper is
// here. Returns { posts: [...], has_more: bool }. Pass `before` (the id of the
// last post you have) to load the next, older page.
export const getUserPosts = (id, before) =>
  request(`/api/users/${id}/posts${before ? `?before=${before}` : ""}`);

// No user ID: the server takes it from the session, so you can only ever
// change your own privacy.
export const setPrivacy = (isPrivate) =>
  request("/api/me/privacy", { method: "PATCH", body: { is_private: isPrivate } });