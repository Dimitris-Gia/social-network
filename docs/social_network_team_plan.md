# Social Network: Team Plan & Timetable

Nov 20, 2026

## Summary

The plan runs 15 working days, from Mon 23 Nov to Fri 11 Dec 2026, with weekends kept as buffer. Each person owns their features from the database to the React page, so everyone does a similar amount of backend and frontend work and can explain a whole feature in the audit.

- **Week 1 (23–27 Nov):** all backend REST endpoints, tested with curl.
- **Week 2 (30 Nov–4 Dec):** all React pages, plus WebSockets for chat and live notifications.
- **Week 3 (7–11 Dec):** Docker, a full run through audit.md, a security cross-review, bonus items and audit rehearsal.

What reading assignment.md and audit.md changed in the first plan:

- **Private profiles must be blocked on the backend**, not only hidden in the UI. The audit checks that a non-follower cannot see a private profile's info, posts or follower lists.
- **Any group member can invite**, not only the creator. The audit asks you to invite one of your *followers*, so the invite picker lists followers.
- **Notifications must be actionable.** Follow requests, group invites and join requests are accepted or refused from the notification itself.
- **Chat is refused when neither user follows the other**, both in the UI and on the server.
- **Auditors open the DB with `sqlite3 <file>.db`**, so the DB file needs a fixed, documented path, mounted as a Docker volume.
- **Cheap bonus points are scheduled in week 3:** a seed migration, confirmation pop-ups for unfollowing and for the privacy toggle, extra notifications, and a build script. OAuth login is left out because it costs too much for the time you have.

## Who owns what

A owns accounts and the social graph, B owns posts and real-time, and C owns groups and events. Each person also owns one piece of infrastructure and the security checks for their own features.

|  | **A: Accounts & social graph** | **B: Posts & real-time** | **C: Groups & events** |
| --- | --- | --- | --- |
| **Tables (migrations)** | users, sessions, follows, notifications | posts, post_allowed_viewers, comments, messages | groups, group_members, events, event_responses |
| **Backend** | Register, login, logout, bcrypt, sessions, auth middleware, profile + privacy toggle, follow/unfollow/requests, followers lists, notifications API + `CreateNotification()`, image upload helper | Posts, comments, 3 privacy levels, feed, WebSocket hub, private chat + history | Create/browse groups, invites, join requests, membership checks, group posts (reusing B's code), events + RSVP, group chat room on B's hub |
| **Frontend** | Login/register, auth context, protected routes, profile page, follow button, user list, notification bell on every page | Post composer, feed, post card + comments, chat UI + emoji picker, unread-message badge | Groups browse/create, group page, invite picker, events + Going/Not going, group chat panel |
| **Infra** | Build script + docker-compose | Backend Dockerfile | `sqlite.go` migration runner, frontend Dockerfile + Caddyfile, seed migration |
| **Security to explain** | bcrypt, HttpOnly/SameSite cookie, session expiry, middleware on every protected route, private-profile gate | Visibility check on *every* post/comment read, WS upgrade authenticated by cookie, chat follow check, image type check | Membership check on every group endpoint (posts, events, chat), only the creator approves join requests |

Dependencies between owners: B and C call A's `CreateNotification()` for invites, join requests and events, and A pushes notifications live through B's `hub.SendToUser()`. C's group posts reuse B's post functions and components, with an extra `group_id`.

## Decisions to agree on day 1

Agree on these before anyone writes a feature. Changing them later costs the most.

**Server, App, Database (audit: "clear separation").** `backend/server.go` is the Go entry point: it opens the DB, runs migrations, builds the routes and listens on :8080. Code under `pkg/handlers` and `pkg/middleware` is the App. `pkg/db` is the Database. Run Caddy as a separate program in the frontend container, where it serves the React build and forwards `/api`, `/ws` and `/uploads` to the backend. **Never import Caddy as a Go module**, because it is not on the allowed-packages list and that is the first audit question.

**Same origin, so cookies just work.** In development, the Vite dev server proxies `/api` and `/ws` to `localhost:8080`. In Docker, Caddy does the same. The browser only ever talks to one origin, so you need no CORS setup and the session cookie is always sent.

**Folder layout:**

```
backend/
  server.go
  pkg/db/migrations/sqlite/000001_create_users_table.up.sql ...
  pkg/db/sqlite/sqlite.go          (connect, foreign_keys=ON, run migrations)
  pkg/models/                      (SQL queries per entity, ? placeholders only)
  pkg/handlers/                    (auth.go, users.go, follows.go, posts.go, groups.go, events.go, notifications.go, chat.go)
  pkg/middleware/auth.go
  pkg/websocket/hub.go, client.go
  pkg/utils/images.go, json.go
  data/social.db, uploads/         (Docker volume, git-ignored)
frontend/
  src/api/  src/context/  src/pages/  src/components/  src/hooks/
  Dockerfile  Caddyfile
```

**Migrations, numbered once on day 1 so nobody's numbers clash:**

| # | Table | Owner |
| --- | --- | --- |
| 000001 | users | A |
| 000002 | sessions | A |
| 000003 | follows (status: pending / accepted) | A |
| 000004 | posts (group_id nullable, privacy) | B |
| 000005 | post_allowed_viewers | B |
| 000006 | comments | B |
| 000007 | groups | C |
| 000008 | group_members (status: invited / requested / member) | C |
| 000009 | events | C |
| 000010 | event_responses | C |
| 000011 | messages (receiver_id or group_id) | B |
| 000012 | notifications | A |
| 000013 | seed data (bonus, week 3) | C |

**API contract.** Keep a shared `API.md` that lists each endpoint with its method, path, request JSON, response JSON and error codes. Each owner writes their own section on day 2. The frontend is built against this file, even before the backend endpoints exist.

## Week 1 (23–27 Nov): backend first

By Friday, every REST endpoint works and has been tested with curl or Postman. Wednesday is the critical day: A's login and middleware must be merged, because every other endpoint depends on them. Until then, B and C use a temporary middleware that sets `userID = 1`.

| Day | A: Accounts | B: Posts & real-time | C: Groups & events |
| --- | --- | --- | --- |
| **Mon 23** (together, morning) | Read assignment.md and audit.md together. Draw the ERD. Agree on everything in "Decisions to agree on day 1" | (same) | (same) |
| **Mon 23** (afternoon) | Write migrations 1–3 and 12. Write `server.go`, the router and JSON helpers | Write migrations 4–6 and 11. Create the React app (Vite) with react-router, the layout (navbar with placeholder bell and chat icons) and an API client using `credentials: "include"`. Set up the Vite proxy | Write migrations 7–10. Write `sqlite.go`: open the DB, turn on foreign keys, run golang-migrate from `file://pkg/db/migrations/sqlite` on start. Check with `sqlite3 data/social.db .tables` |
| **Tue 24** | Write your part of API.md. Register: validate the required fields, bcrypt the password, return 409 if the email exists. Write `utils/images.go`: accept JPEG/PNG/GIF only (check with `http.DetectContentType`), cap the size, save with a UUID filename | Write your part of API.md. Create post, with the image helper once A pushes it. Write `CanViewPost(viewerID, post)` covering all 3 privacy levels | Write your part of API.md. Create group, browse all groups, get one group |
| **Wed 25** | Login: one generic error for a wrong email *or* a wrong password. Logout. Session middleware: UUID in an HttpOnly, SameSite=Lax cookie, looked up in the DB, with expiry. `GET /api/me`. **Merge by evening** | Comments (with image). Feed query: public posts + followers' "almost private" posts + private posts where you are an allowed viewer + your own posts | Invites: any member can invite, and the invitee accepts or declines. Join requests: only the creator accepts or refuses |
| **Thu 26** | Profile GET, gated: self, public profile, or accepted follower. Privacy toggle. Follow (public = accepted at once, private = pending), unfollow, accept/decline, followers/following lists (gated the same way) | Switch to A's real middleware. Allowed viewers: check that each chosen user really follows you. `GET /users/{id}/posts`, gated by the profile rule and then the post rule | `IsMember(userID, groupID)` helper used on every group route. Group posts and comments, calling B's post functions with `group_id` |
| **Fri 27** | Notifications: list, unread count, mark read, `CreateNotification()`. Hook it into follow requests. Accept/decline from the notification. Start the login page | Start the frontend: feed page and PostCard component, using real endpoints | Events: create (title, description, date/time), Going/Not going, list per group. Notify every member on event creation; notify on invites and join requests |
| **Sat–Sun** | Buffer. Merge to `main`, and run a curl smoke test of every endpoint | (same) | (same) |

## Week 2 (30 Nov–4 Dec): frontend and real-time

By Friday, every feature works end to end on your own machines. B must have the WebSocket hub merged by Monday evening, because A's live notifications and C's group chat both run on it. The app opens one WS connection per user.

| Day | A: Accounts | B: Posts & real-time | C: Groups & events |
| --- | --- | --- | --- |
| **Mon 30** | Register page with all 8 fields (the 3 optional ones visible but skippable). AuthContext built on `/api/me`, protected routes, logout always visible in the navbar | `pkg/websocket`: hub with register/unregister, `SendToUser()` and `SendToGroup()`. Authenticate the upgrade with the session cookie. Frontend: one `WebSocketProvider` context. **Merge by evening** | Groups browse page with a request-to-join button. Create-group form (title, description) |
| **Tue 1** | Profile page: all register info except password, the user's posts (B's PostCard), followers/following tabs, privacy toggle on your own profile | Private chat backend: save the message, check the follow (either direction), history endpoint, "who can I chat with" endpoint. A non-follower message is refused and logged, without crashing | Group page: members, posts and events tabs. Invite picker listing *your followers*. Button states (Invited / Requested / Member) |
| **Wed 2** | Follow button states (Follow / Requested / Unfollow). "This account is private" view. Users list page to find people | Chat UI: contact list (allowed users only), conversation view, live receive, emoji picker (a small emoji grid is enough) | Group posts and comments UI, reusing B's Composer and PostCard with a `groupId` prop |
| **Thu 3** | Notification bell in the navbar on every page: unread badge, dropdown, inline Accept/Decline for follow requests. Live push through `hub.SendToUser` | Post composer: privacy selector, follower checklist for "private", image upload with preview. Comments UI with image | Events UI: create form (`datetime-local`), event list, Going/Not going with counts |
| **Fri 4** | Pair with C: Accept/Decline in the bell for group invites and join requests. Bonus: confirmation pop-ups for unfollowing and for the privacy toggle | Unread-messages badge on the chat icon, **separate from the bell** (the audit checks this). Fix chat bugs found while testing in 2 browsers | Group chat: room on the hub, membership checked on every send, chat panel on the group page. Test with 3 browsers |
| **Sat–Sun** | Buffer. Merge everything and click through the whole app | (same) | (same) |

## Week 3 (7–11 Dec): Docker, audit run, cross-review

No new features this week, only Docker, fixes, bonus items and rehearsal. On Friday the team freezes a build that passes every audit.md question from a clean Docker start.

| Day | A: Accounts | B: Posts & real-time | C: Groups & events |
| --- | --- | --- | --- |
| **Mon 7** | docker-compose and `build.sh` (bonus: one script builds and starts both images). Bonus: extra notifications such as "request accepted" and "new follower" | Backend Dockerfile, multi-stage. **mattn/go-sqlite3 needs CGO:** build with `CGO_ENABLED=1` and gcc in the build stage. Volumes for `data/` and `uploads/` | Frontend Dockerfile: Node build stage, then a Caddy stage serving `dist/` and forwarding `/api`, `/ws` and `/uploads` to the backend container |
| **Tue 8** | All: run audit.md top to bottom in Docker with 3 browsers (Chrome, Firefox, private window). Log every failure in a shared list. Fix your own items | (same) | (same) |
| **Wed 9** | Review **C's** code: is there a membership check on every group, event and group-chat route? Can a non-member read group posts by ID? | Review **A's** code: cookie flags, session expiry, logout deletes the session, private-profile gate on profile, posts and followers endpoints | Review **B's** code: `CanViewPost` on single post and comment reads (not only the feed), WS auth, chat follow check, image type check |
| **Wed 9** (all) | Everyone checks their own SQL: `?` placeholders only, no string concatenation. In React: no `dangerouslySetInnerHTML` | (same) | (same) |
| **Thu 10** | Fix review findings. Write the README section on how to run the project | Fix review findings. CSS polish and responsiveness for feed and chat | Seed migration 000013 (bonus): a few users, posts and a group for the auditors. Fix review findings |
| **Fri 11** | Rehearsal: explain **B's** features from the click to the handler to the SQL | Explain **C's** features | Explain **A's** features |
| **Fri 11** (all) | `docker compose down -v`, rebuild from scratch, run the audit one last time. Tag the release | (same) | (same) |

## Audit checklist by owner

Every audit.md section has one owner, who makes sure it passes on Tuesday 8 Dec. The right-hand column lists the audit question most likely to catch you out in that section.

| Audit section | Owner | Easy to miss |
| --- | --- | --- |
| Allowed packages, file structure | C | No Caddy Go import. The migrations folder matches the subject's layout exactly |
| Backend: server / app / database | A | Be able to point at `server.go`, `pkg/handlers`, `pkg/db` |
| Database + migrations applied | C | Auditors open the DB with `sqlite3`, so know its path inside the container |
| Authentication | A | Duplicate email detected. Two browsers keep their own users after refresh |
| Followers | A | Private = request, public = instant. Accept/decline works from a second browser |
| Profile | A | Non-follower blocked from a private profile, on the backend |
| Posts | B | Image *and* GIF on comments too. Private post's viewer picker |
| Groups + events | C | A non-creator member can invite. Event has title, description, date/time, 2 options |
| Chat | B (group chat: C) | Not following each other = no chat. Only the target receives. 3 browsers in group chat. Emojis |
| Notifications | A | Visible on every page, separate from messages. All 4 required types |
| Docker | B | Two containers with non-zero size, `docker ps -a`, reachable in the browser |

**Bonus items in the plan:** seed migration (C), unfollow pop-up and privacy pop-up (A), extra notifications (A), build script (A). OAuth is skipped. It could be built with the standard library only, but it would take about 2 days you don't have.

## Risks and the 2-week fallback

If you only get 2 weeks, keep Week 1 as it is. Squeeze Week 2 into Mon–Thu, and use Fri 4 Dec for Docker, the audit run and a short cross-explanation. Drop every bonus except the two confirmation pop-ups, which take about an hour.

| Risk | What to do |
| --- | --- |
| A's login is late and blocks everyone | B and C use the temporary `userID = 1` middleware. A's only Wednesday goal is login + middleware |
| C has the most permission rules and falls behind | B takes over group chat, since B wrote the hub |
| Session cookie not sent during development | Use the Vite proxy (same origin). Don't set up CORS |
| Docker build fails on sqlite3 | Turn CGO on and install gcc in the build stage. Test the Dockerfile on Mon 7, not on Friday |
| A member can't explain another person's code | Do the Wed 9 cross-review and the Fri 11 rehearsal. Auditors ask anyone about anything |