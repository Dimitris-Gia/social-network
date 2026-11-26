// Owner: A — info, posts, followers / following, privacy toggle
//
// Route: /users/:id (your own profile is just /users/<your id>, the navbar
// links there). The SERVER decides what we may see: GET /api/users/{id}
// returns can_view, and when it's false the private fields are not in the
// response at all. This page only reacts to that; it never hides data that
// it received.
import { useEffect, useState } from "react";
import { useParams } from "react-router-dom";
import * as usersApi from "../api/users.js";
import useAuth from "../hooks/useAuth.js";
import Avatar, { displayName } from "../components/common/Avatar.jsx";
import UserCard from "../components/users/UserCard.jsx";
import PostCard from "../components/posts/PostCard.jsx";

export default function ProfilePage() {
  const { id } = useParams();
  const [profile, setProfile] = useState(null);
  const [error, setError] = useState("");
  const [tab, setTab] = useState("posts");

  // Load the profile every time the :id in the URL changes. Clicking a name
  // in the followers tab goes from /users/3 to /users/7: React Router keeps
  // the SAME ProfilePage mounted and only changes `id`, so this effect must
  // depend on [id] and reset the old state.
  useEffect(() => {
    // If you click quickly from profile 3 to profile 7, the answer for 3 can
    // arrive AFTER the one for 7. `ignore` makes the cleanup of the old effect
    // throw that late answer away, so you never see the wrong person.
    let ignore = false;
    setProfile(null);
    setError("");
    setTab("posts");

    usersApi
      .getUser(id)
      .then((u) => !ignore && setProfile(u))
      .catch((err) => !ignore && setError(err.message));

    return () => {
      ignore = true;
    };
  }, [id]);

  // After the privacy toggle, reload: going public auto-accepts pending
  // requests on the server, so the follower count may have changed too.
  async function reload() {
    setProfile(await usersApi.getUser(id));
  }

  if (error) return <p className="page-loading">{error}</p>; // e.g. "user not found"
  if (!profile) return <p className="page-loading">Loading…</p>;

  const isMe = profile.follow_status === "self";

  return (
    <section className="profile-page">
      <header className="profile-header">
        <Avatar user={profile} size={96} />
        <div>
          <h1>{displayName(profile)}</h1>
          {profile.nickname && (
            <p className="profile-muted">
              {profile.first_name} {profile.last_name}
            </p>
          )}
          <p className="profile-muted">{profile.is_private ? "🔒 Private profile" : "🌐 Public profile"}</p>
        </div>
        {/* Wed 2: <FollowButton> goes here for other people's profiles. */}
        {isMe && <PrivacyToggle isPrivate={profile.is_private} onChanged={reload} />}
      </header>

      {!profile.can_view ? (
        // The server only sent id, names, avatar, is_private, follow_status.
        // Wed 2 turns this into the full "This account is private" view.
        <p className="profile-locked">This account is private. Follow them to see their posts and info.</p>
      ) : (
        <>
          <AboutSection profile={profile} />

          <nav className="profile-tabs" aria-label="Profile sections">
            <TabButton name="posts" tab={tab} setTab={setTab}>Posts</TabButton>
            <TabButton name="followers" tab={tab} setTab={setTab}>
              Followers ({profile.followers_count})
            </TabButton>
            <TabButton name="following" tab={tab} setTab={setTab}>
              Following ({profile.following_count})
            </TabButton>
          </nav>

          {/* key={id}: a new user gets a fresh tab component with empty state. */}
          {tab === "posts" && <PostsTab key={id} userId={id} />}
          {tab === "followers" && (
            <UserListTab key={`followers-${id}`} load={() => usersApi.getFollowers(id)} empty="No followers yet." />
          )}
          {tab === "following" && (
            <UserListTab key={`following-${id}`} load={() => usersApi.getFollowing(id)} empty="Not following anyone yet." />
          )}
        </>
      )}
    </section>
  );
}

// Everything from the register form except the password (audit: Profile).
// Every value is rendered as text, so React escapes it: an about_me of
// "<script>…</script>" shows up as those characters and never runs.
function AboutSection({ profile }) {
  return (
    <dl className="profile-about">
      <dt>Email</dt>
      <dd>{profile.email}</dd>
      <dt>First name</dt>
      <dd>{profile.first_name}</dd>
      <dt>Last name</dt>
      <dd>{profile.last_name}</dd>
      <dt>Date of birth</dt>
      <dd>{formatBirthday(profile.date_of_birth)}</dd>
      <dt>Nickname</dt>
      <dd>{profile.nickname || "—"}</dd>
      <dt>About me</dt>
      <dd className="profile-about-me">{profile.about_me || "—"}</dd>
      <dt>Joined</dt>
      <dd>{new Date(profile.created_at).toLocaleDateString()}</dd>
    </dl>
  );
}

// "2001-04-17" -> "17 April 2001" in the browser's language.
// new Date("2001-04-17") would mean MIDNIGHT UTC, which is still the 16th in
// America, so we build the date from its parts in local time instead.
function formatBirthday(ymd) {
  const [y, m, d] = ymd.split("-").map(Number);
  return new Date(y, m - 1, d).toLocaleDateString(undefined, { day: "numeric", month: "long", year: "numeric" });
}

function TabButton({ name, tab, setTab, children }) {
  return (
    <button type="button" aria-pressed={tab === name} onClick={() => setTab(name)}>
      {children}
    </button>
  );
}

// Only shown on your own profile. PATCH /api/me/privacy has no user ID in
// the URL, so even a hand-made request can only change YOUR privacy.
function PrivacyToggle({ isPrivate, onChanged }) {
  const { setUser } = useAuth();
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  async function toggle() {
    // Bonus (Fri 4): replace with <ConfirmDialog>.
    setSaving(true);
    setError("");
    try {
      const res = await usersApi.setPrivacy(!isPrivate);
      // Keep AuthContext in sync, so other pages that read user.is_private
      // don't show the old value.
      setUser((u) => ({ ...u, is_private: res.is_private }));
      await onChanged();
    } catch (err) {
      setError(err.message);
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="privacy-toggle">
      <button type="button" onClick={toggle} disabled={saving}>
        {isPrivate ? "Make profile public" : "Make profile private"}
      </button>
      {error && <p className="form-error">{error}</p>}
    </div>
  );
}

// The user's posts, newest first, with "Load more" (cursor pagination:
// ?before=<id of the oldest post we have>). Each post's own privacy is
// checked on the server by B's CanViewPost, so an "almost private" or
// "private" post you may not see is simply not in the list.
function PostsTab({ userId }) {
  const [posts, setPosts] = useState([]);
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  async function load(before) {
    setLoading(true);
    setError("");
    try {
      const page = await usersApi.getUserPosts(userId, before);
      setPosts((old) => (before ? [...old, ...page.posts] : page.posts));
      setHasMore(page.has_more);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []); // the parent gives this component a new key when userId changes

  if (error) return <p className="form-error">{error}</p>;
  if (!loading && posts.length === 0) return <p className="profile-muted">No posts yet.</p>;

  return (
    <div className="profile-posts">
      {posts.map((p) => (
        <PostCard key={p.id} post={p} />
      ))}
      {loading && <p className="profile-muted">Loading…</p>}
      {hasMore && !loading && (
        <button type="button" onClick={() => load(posts[posts.length - 1].id)}>
          Load more
        </button>
      )}
    </div>
  );
}

// Followers or following: same list, different endpoint, so the parent
// passes `load`. Both endpoints answer 403 for a private profile you don't
// follow, but we never get here in that case (can_view is false above).
function UserListTab({ load, empty }) {
  const [users, setUsers] = useState(null);
  const [error, setError] = useState("");

  useEffect(() => {
    let ignore = false;
    load()
      .then((list) => !ignore && setUsers(list))
      .catch((err) => !ignore && setError(err.message));
    return () => {
      ignore = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []); // new key from the parent = new list

  if (error) return <p className="form-error">{error}</p>;
  if (!users) return <p className="profile-muted">Loading…</p>;
  if (users.length === 0) return <p className="profile-muted">{empty}</p>;

  return (
    <div className="user-list">
      {users.map((u) => (
        <UserCard key={u.id} user={u} />
      ))}
    </div>
  );
}