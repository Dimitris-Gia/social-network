# API contract

One section per owner. For each endpoint: method, path, request JSON, response JSON, error codes.

## A: Accounts & social graph

TODO

## B: Posts & real-time

TODO

## C: Groups & events

All routes require authentication unless stated otherwise. User identity comes from the session cookie.

### Group creation and lists

- `POST /api/groups`
  - Request: `{"title":"...","description":"..."}`
  - Response: `201` with the created group object
  - Errors: `400` invalid fields, `401` user is not authenticated
- `GET /api/groups`
  - Response: `200` with a list of all groups and the current user's status (`member`, `invited`, `requested`, `none`)
- `GET /api/groups/{id}`
  - Response: `200` with the group, creator, members, and current user's status
  - Errors: `404` group does not exist

### Join requests and invitations

- `POST /api/groups/{id}/join`
  - Request: `{}` or no body
  - Response: `201` with `{}` after creating a `requested` status
  - Errors: `409` a request, invitation, or membership already exists
- `POST /api/groups/{id}/invite`
  - Request: `{"user_id":2}`
  - Response: `201` with `invited` status
  - Errors: `403` the caller is not a member, `409` the target is already a member or invited
  - Any group member can invite one of their followers
- `POST /api/groups/{id}/members/{user_id}/accept`
  - Request: `{}`
  - Response: `200` with `{}` after setting status to `member`
  - Invitations: the invited user accepts.
  - Join requests: only the group creator accepts.
- `POST /api/groups/{id}/members/{user_id}/decline`
  - Request: `{}`
  - Response: `204`
  - Invitations: the invited user declines.
  - Join requests: only the group creator declines.
- `GET /api/groups/{id}/members`
  - Response: `200` with confirmed members only
  - Errors: `403` the requester is not a member

### Events

- `POST /api/groups/{id}/events`
  - Request: `{"title":"...","description":"...","event_time":"2026-12-01T18:30"}`
  - Response: `201` with the created event
  - Only a member can create an event.
- `GET /api/groups/{id}/events`
  - Response: `200` with events, `going` and `not_going` counts, and the current user's response
  - Errors: `403` the requester is not a member
- `POST /api/groups/{id}/events/{event_id}/response`
  - Request: `{"response":"going"}` or `{"response":"not_going"}`
  - Response: `200` with updated counts
  - Only a member can respond. The user can change their existing response.

Common errors: `401 Unauthorized`, `403 Forbidden`, `404 Not Found`, `409 Conflict`, `500 Internal Server Error`.
