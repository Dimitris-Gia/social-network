# Social Network: Folder Structure

Nov 23, 2026. Agreed on day 1. The `backend/pkg/db/...` part follows the subject's tree exactly, because the audit tests the file organisation and that migrations apply from it. Initials show who creates each file.

## Repository root

```
social-network/
├── backend/                    Go: server + app + database
├── frontend/                   React (Vite) + Caddy
├── docs/
│   ├── ERD.md                  the ERD (Mermaid)
│   └── API.md                  endpoint contract, one section per owner
├── docker-compose.yml          A   both images, ports, volumes
├── build.sh                    A   bonus: build + start everything in one command
├── .gitignore                      backend/data/, backend/uploads/, node_modules/, dist/
└── README.md                   A   how to run (dev and Docker), DB path for auditors
```

## Backend

```
backend/
├── server.go                   A   entry point: open DB, run migrations, create hub,
│                                   register routes, serve /uploads, listen on :8080
├── go.mod / go.sum                 allowed packages only
├── Dockerfile                  B   multi-stage, CGO_ENABLED=1 (go-sqlite3 needs gcc)
├── .dockerignore
│
├── pkg/
│   ├── db/                         ── DATABASE ──
│   │   ├── migrations/
│   │   │   └── sqlite/
│   │   │       ├── 000001_create_users_table.up.sql                  A
│   │   │       ├── 000001_create_users_table.down.sql                A
│   │   │       ├── 000002_create_sessions_table.up.sql / .down.sql   A
│   │   │       ├── 000003_create_follows_table.up.sql / .down.sql    A
│   │   │       ├── 000004_create_posts_table.up.sql / .down.sql      B
│   │   │       ├── 000005_create_post_allowed_viewers_table...       B
│   │   │       ├── 000006_create_comments_table...                   B
│   │   │       ├── 000007_create_groups_table...                     C
│   │   │       ├── 000008_create_group_members_table...              C
│   │   │       ├── 000009_create_events_table...                     C
│   │   │       ├── 000010_create_event_responses_table...            C
│   │   │       ├── 000011_create_messages_table...                   B
│   │   │       ├── 000012_create_notifications_table...              A
│   │   │       └── 000013_seed_data...                               C  (week 3)
│   │   └── sqlite/
│   │       └── sqlite.go       C   Open(): connect with _foreign_keys=on,
│   │                               run golang-migrate from file://pkg/db/migrations/sqlite
│   │
│   ├── models/                     SQL per entity, `?` placeholders only, no HTTP code
│   │   ├── user.go             A   CreateUser, GetUserByEmail, GetUserByID, SetPrivacy
│   │   ├── session.go          A   CreateSession, GetUserIDBySession, DeleteSession
│   │   ├── follow.go           A   Follow, Accept, Delete, IsFollowing, Followers, Following
│   │   ├── notification.go     A   CreateNotification, List, UnreadCount, MarkRead
│   │   ├── post.go             B   CreatePost, GetPost, Feed, PostsByUser, CanViewPost
│   │   ├── comment.go          B   CreateComment, CommentsByPost
│   │   ├── message.go          B   SaveMessage, History, CanChat, UnreadCount
│   │   ├── group.go            C   CreateGroup, ListGroups, GetGroup, IsMember,
│   │   │                           Invite, RequestJoin, AcceptMember, Members
│   │   └── event.go            C   CreateEvent, EventsByGroup, Respond
│   │
│   ├── handlers/                   ── APP ── decode JSON, check permissions, call models
│   │   ├── routes.go           A   RegisterRoutes(mux, db, hub): every route in one place,
│   │   │                           so you can see which ones are wrapped in RequireAuth
│   │   ├── auth.go             A   POST /api/register, /api/login, /api/logout, GET /api/me
│   │   ├── users.go            A   GET /api/users, /api/users/{id}, PATCH privacy
│   │   ├── follows.go          A   follow / unfollow / accept / decline, follower lists
│   │   ├── notifications.go    A   list, unread count, mark read
│   │   ├── posts.go            B   create post, feed, single post, user's posts
│   │   ├── comments.go         B   create + list comments
│   │   ├── chat.go             B   GET /ws upgrade, history, "who can I chat with"
│   │   ├── groups.go           C   groups, invites, join requests, group posts
│   │   └── events.go           C   create event, list, Going / Not going
│   │
│   ├── middleware/
│   │   └── auth.go             A   RequireAuth: read cookie → sessions table → userID in
│   │                               request context, else 401
│   │
│   ├── websocket/
│   │   ├── hub.go              B   map userID → clients; SendToUser, SendToGroup
│   │   └── client.go           B   readPump / writePump per connection (gorilla/websocket)
│   │
│   └── utils/
│       ├── json.go             A   WriteJSON, WriteError, ReadJSON
│       └── images.go           A   SaveImage: DetectContentType → jpeg/png/gif only,
│                                   size cap, UUID filename, returns the path
│
├── data/                           git-ignored, Docker volume
│   └── social.db                   created on first start
└── uploads/                        git-ignored, Docker volume, served at /uploads/
```

**Server / App / Database, for the audit question:** `server.go` (+ Caddy in the frontend container) is the server. `pkg/handlers`, `pkg/middleware`, `pkg/websocket`, `pkg/utils` are the app. `pkg/db` + `pkg/models` are the database layer.

**Rule that keeps the layers clean:** handlers never write SQL, models never touch `http.Request`. A handler gets `userID` from the context, asks a model "may this user see this?", then answers with JSON.

## Frontend

JavaScript + JSX (no TypeScript, to keep the first framework project simple).

```
frontend/
├── index.html
├── package.json                    react, react-dom, react-router-dom
├── vite.config.js              B   dev proxy: /api, /ws, /uploads → localhost:8080
├── Dockerfile                  C   stage 1 node: npm run build; stage 2 caddy: serve dist/
├── Caddyfile                   C   serve dist/, SPA fallback to index.html,
│                                   reverse_proxy /api/* /ws /uploads/* → backend:8080
├── .dockerignore
│
└── src/
    ├── main.jsx                B   mounts <App/> inside the providers
    ├── App.jsx                 B   react-router routes, layout with Navbar
    ├── index.css
    │
    ├── api/                        one file per area; all go through client.js
    │   ├── client.js           B   fetch wrapper: credentials "include", JSON, errors
    │   ├── auth.js             A
    │   ├── users.js            A   profiles, follows
    │   ├── notifications.js    A
    │   ├── posts.js            B   posts + comments
    │   ├── chat.js             B
    │   └── groups.js           C   groups + events
    │
    ├── context/
    │   ├── AuthContext.jsx     A   current user from GET /api/me, login(), logout()
    │   └── WebSocketContext.jsx B  one WS per logged-in user, subscribe(type, handler)
    │
    ├── hooks/
    │   ├── useAuth.js          A
    │   ├── useNotifications.js A   list + unread count, live updates from WS
    │   └── useChat.js          B   messages + unread count, live updates from WS
    │
    ├── components/
    │   ├── layout/
    │   │   ├── Navbar.jsx      B   links, NotificationBell, ChatIcon, logout (A wires it)
    │   │   └── ProtectedRoute.jsx A redirect to /login if not logged in
    │   ├── common/
    │   │   ├── Avatar.jsx      A
    │   │   ├── ImageInput.jsx  A   file picker, accepts image/jpeg,png,gif, preview
    │   │   └── ConfirmDialog.jsx A bonus: unfollow + privacy pop-ups
    │   ├── users/
    │   │   ├── FollowButton.jsx A  Follow / Requested / Unfollow
    │   │   └── UserCard.jsx    A
    │   ├── notifications/
    │   │   ├── NotificationBell.jsx A badge + dropdown (on every page via Navbar)
    │   │   └── NotificationItem.jsx A inline Accept / Decline
    │   ├── posts/
    │   │   ├── PostComposer.jsx B  text, image, PrivacySelector; takes optional groupId
    │   │   ├── PrivacySelector.jsx B public / almost private / private + follower checklist
    │   │   ├── PostCard.jsx    B
    │   │   └── CommentSection.jsx B
    │   ├── chat/
    │   │   ├── ChatIcon.jsx    B   unread-messages badge, separate from the bell
    │   │   ├── ChatWindow.jsx  B   used for private chat and by C for group chat
    │   │   └── EmojiPicker.jsx B
    │   └── groups/
    │       ├── GroupCard.jsx   C   join button: Request / Requested / Member
    │       ├── InvitePicker.jsx C  lists your followers
    │       ├── EventForm.jsx   C   title, description, datetime-local
    │       └── EventCard.jsx   C   Going / Not going + counts
    │
    └── pages/
        ├── LoginPage.jsx       A
        ├── RegisterPage.jsx    A   8 fields, 3 optional
        ├── ProfilePage.jsx     A   info, posts, followers / following, privacy toggle
        ├── UsersPage.jsx       A   find people to follow
        ├── FeedPage.jsx        B
        ├── ChatPage.jsx        B
        ├── GroupsPage.jsx      C   browse + create
        ├── GroupPage.jsx       C   members, posts, events, chat tabs
        └── NotFoundPage.jsx    B
```

## Paths inside Docker

| What | Path in backend container | Volume |
| --- | --- | --- |
| Database | `/app/data/social.db` | `db-data:/app/data` |
| Uploaded images | `/app/uploads/` | `uploads:/app/uploads` |
| Migrations | `/app/pkg/db/migrations/sqlite` (copied into the image) | none |

The backend container's working directory is `/app`, so the same relative path `file://pkg/db/migrations/sqlite` works locally (run from `backend/`) and in Docker. Auditor command: `docker exec -it backend sqlite3 /app/data/social.db .tables` (install the `sqlite` package in the runtime stage so this works).
