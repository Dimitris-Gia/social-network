// Owner: A — list, unread count, mark read (API.md 1.4)
import { request } from "./client.js";

export const listNotifications = () => request("/api/notifications");

// -> { count: 3 }
export const unreadNotificationCount = () => request("/api/notifications/unread-count");

export const markNotificationRead = (id) =>
  request(`/api/notifications/${id}/read`, { method: "POST" });

export const markAllNotificationsRead = () =>
  request("/api/notifications/read-all", { method: "POST" });

// The bell's Accept / Decline for a follow_request call these (actor.id is
// the requester). Group invites and join requests use C's endpoints.
export const acceptFollowRequest = (userId) =>
  request(`/api/follow-requests/${userId}/accept`, { method: "POST" });

export const declineFollowRequest = (userId) =>
  request(`/api/follow-requests/${userId}/decline`, { method: "POST" });