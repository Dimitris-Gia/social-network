// Owner: C — groups + events

import { request } from "./client.js";

export function listGroups() {
  return request("/api/groups");
}

export function createGroup(title, description) {
  return request("/api/groups", {
    method: "POST",
    body: { title, description },
  });
}

export function getGroup(groupId) {
  return request(`/api/groups/${groupId}`);
}

export function joinGroup(groupId) {
  return request(`/api/groups/${groupId}/join`, {
    method: "POST",
  });
}

export function inviteToGroup(groupId, userId) {
  return request(`/api/groups/${groupId}/invite`, {
    method: "POST",
    body: { user_id: userId },
  });
}

export function getGroupMembers(groupId) {
  return request(`/api/groups/${groupId}/members`);
}

export function acceptGroupMember(groupId, userId) {
  return request(`/api/groups/${groupId}/members/${userId}/accept`, {
    method: "POST",
  });
}

export function declineGroupMember(groupId, userId) {
  return request(`/api/groups/${groupId}/members/${userId}/decline`, {
    method: "POST",
  });
}

export function listEvents(groupId) {
  return request(`/api/groups/${groupId}/events`);
}

export function createEvent(groupId, title, description, eventTime) {
  return request(`/api/groups/${groupId}/events`, {
    method: "POST",
    body: {
      title,
      description,
      event_time: eventTime,
    },
  });
}

export function respondToEvent(groupId, eventId, response) {
  return request(`/api/groups/${groupId}/events/${eventId}/response`, {
    method: "POST",
    body: { response },
  });
}

export function listGroupPosts(groupId, before = 0) {
  const query = before > 0 ? `?before=${before}` : "";
  return request(`/api/groups/${groupId}/posts${query}`);
}

export function createGroupPost(groupId, formData) {
  return request(`/api/groups/${groupId}/posts`, {
    method: "POST",
    body: formData,
  });
}

export function listGroupMessages(groupId, before = 0) {
  const query = before > 0 ? `?before=${before}` : "";
  return request(`/api/groups/${groupId}/messages${query}`);
}
