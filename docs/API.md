# API contract

One section per owner. For each endpoint: method, path, request JSON, response JSON, error codes.

## A: Accounts & social graph

TODO

## B: Posts & real-time

### Shared objects
 
**UserSummary**: the small user object embedded in posts, comments and messages.
 
```json
{ "id": 3, "first_name": "Ada", "last_name": "Lovelace", "nickname": "ada", "avatar_path": "/uploads/3f2a....png" }
```
 
`nickname` and `avatar_path` are `null` when the user has not set them.
 
**Post**
 
```json
{
  "id": 12,
  "author": { "...UserSummary" },
  "group_id": null,
  "content": "Hello world",
  "image_path": "/uploads/9c1e....gif",
  "privacy": "public",
  "comment_count": 2,
  "created_at": "2026-11-25T14:30:00Z"
}
```
 
- `privacy` is `"public"`, `"almost_private"` or `"private"`.
- `group_id` is `null` for a normal post. For a group post it holds the group ID, and `privacy` is ignored (see C's section).
- `image_path` is `null` when there is no image.
**Comment**
 
```json
{
  "id": 7,
  "post_id": 12,
  "author": { "...UserSummary" },
  "content": "Nice!",
  "image_path": null,
  "created_at": "2026-11-25T15:02:00Z"
}
```
 
**Message**
 
```json
{
  "id": 40,
  "sender_id": 3,
  "receiver_id": 5,
  "content": "Hi 😀",
  "is_read": false,
  "created_at": "2026-11-27T09:15:00Z"
}
```
 
### Who can see a post
 
The server checks this on **every** read: feed, single post, a user's posts, and the comments of a post.
 
| Post | Visible to |
| --- | --- |
| Your own post | You, always |
| `public` | Every logged-in user |
| `almost_private` | The author's accepted followers |
| `private` | Only users listed in `allowed_viewers` |
| Group post (`group_id` set) | Members of that group only (checked first, `privacy` is ignored) |
 
A user's posts (`GET /api/users/{id}/posts`) are also gated by the profile rule: a private profile hides its posts from non-followers, whatever the post's own privacy.
 
### Posts
 
#### `POST /api/posts`: create a post
 
`multipart/form-data` (so an image can be attached):
 
| Field | Required | Notes |
| --- | --- | --- |
| `content` | yes | Text, 1 to 5000 characters, trimmed |
| `privacy` | no | `public` (default), `almost_private` or `private` |
| `allowed_viewers` | only if `privacy=private` | Comma-separated user IDs, e.g. `4,9,12`. At least one. Every ID must be an accepted follower of the author |
| `image` | no | JPEG, PNG or GIF, max 5 MB |
 
Response `201`: the created **Post**.
 
Errors: `400` empty content, bad privacy value, `private` with no viewers, or a viewer who does not follow you. `413` image too large. `415` wrong image type.
 
#### `GET /api/posts/feed`: my feed
 
Query: `limit` (default 20, max 50) and `before` (a post ID; return only older posts, for "load more").
 
The feed contains: your own posts, public posts, `almost_private` posts from users you follow, and `private` posts where you are an allowed viewer. Group posts are **not** in the feed. Newest first.
 
Response `200`:
 
```json
{ "posts": [ { "...Post" } ], "has_more": true }
```
 
#### `GET /api/posts/{id}`: one post
 
Response `200`: a **Post**. Errors: `404` if it does not exist **or** you are not allowed to see it.
 
#### `GET /api/users/{id}/posts`: posts on a profile
 
Same query parameters and response shape as the feed. Used by the profile page.
 
Returns only posts you are allowed to see. If the profile is private and you are not the owner or an accepted follower, the answer is `403`. Group posts are not included.
 
Errors: `403` private profile, `404` unknown user.
 
### Comments
 
#### `POST /api/posts/{id}/comments`: add a comment
 
`multipart/form-data`:
 
| Field | Required | Notes |
| --- | --- | --- |
| `content` | yes | Text, 1 to 2000 characters, trimmed |
| `image` | no | JPEG, PNG or GIF, max 5 MB |
 
Response `201`: the created **Comment**.
 
Errors: `400` empty content, `404` post missing or not visible to you, `413`, `415`.
 
#### `GET /api/posts/{id}/comments`: list comments
 
Oldest first. Response `200`:
 
```json
{ "comments": [ { "...Comment" } ] }
```
 
Errors: `404` post missing or not visible to you.
 
### Chat (private)
 
You may chat with a user if **at least one of you follows the other** (accepted follow). The server enforces this, not only the UI.
 
#### `GET /api/chat/contacts`: who can I chat with
 
Response `200`, most recent conversation first:
 
```json
{
  "contacts": [
    {
      "user": { "...UserSummary" },
      "last_message": { "...Message" },
      "unread_count": 2
    }
  ]
}
```
 
`last_message` is `null` if you have never written to each other.
 
#### `GET /api/chat/{userID}/messages`: conversation history
 
Query: `limit` (default 30, max 100) and `before` (a message ID, for scrolling up).
 
Response `200`, oldest first:
 
```json
{ "messages": [ { "...Message" } ], "has_more": false }
```
 
Errors: `403` you are not allowed to chat with this user, `404` unknown user.
 
#### `POST /api/chat/{userID}/read`: mark a conversation as read
 
No body. Marks every message from `{userID}` to you as read. Response `204`.
 
Errors: `403`, `404` as above.
 
#### `GET /api/chat/unread-count`: unread badge
 
Response `200`:
 
```json
{ "unread_count": 3 }
```
 
This counts private messages only. It is **separate from notifications** (A's bell).
 
### WebSocket: `GET /ws`
 
One connection per logged-in user, opened once by `WebSocketProvider` after login. The upgrade is authenticated with the session cookie. No valid cookie returns `401` and the connection is not upgraded.
 
**Every frame in both directions is JSON with this envelope**, shared by B (chat), A (notifications) and C (group chat):
 
```json
{ "type": "<event name>", "data": { } }
```
 
#### Client to server
 
| `type` | `data` | Effect |
| --- | --- | --- |
| `private_message` | `{ "to": 5, "content": "Hi 😀" }` | Saves the message and delivers it to user 5 |
 
`content` is 1 to 2000 characters. Emojis are plain UTF-8 text.
 
#### Server to client
 
| `type` | `data` | When |
| --- | --- | --- |
| `private_message` | a **Message** | You received a message. Also sent back to the sender's own connection(s), so a second tab stays in sync |
| `error` | `{ "message": "..." }` | A frame was refused (not allowed to chat, empty content, bad JSON). The connection **stays open** |
| `notification` | defined by A | New notification (sent by A through `hub.SendToUser`) |
| `group_message` | defined by C | New group chat message (sent by C through `hub.SendToGroup`) |
 
A refused message is logged on the server and never crashes it. Only the target user receives a private message, nobody else.
 
### Images
 
Uploaded images are saved by A's `utils.SaveImage` under a random UUID filename and served from `/uploads/<file>`. The API returns `image_path` as that URL path (for example `/uploads/9c1e....gif`), ready to use in an `<img src>`. The database stores `uploads/<file>`; the handler adds the leading `/`.


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
