import { useEffect, useState } from "react";
import { request } from "../../api/client.js";

// Owner: C — lists your followers
export default function InvitePicker({ groupId, inviterId, members, onInvite }) {
  const [followers, setFollowers] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    let ignore = false;

    async function loadFollowers() {
      try {
        setLoading(true);
        setError("");
        const list = await request(`/api/users/${inviterId}/followers`);
        if (!ignore) setFollowers(Array.isArray(list) ? list : []);
      } catch (loadError) {
        if (!ignore) setError(loadError.message);
      } finally {
        if (!ignore) setLoading(false);
      }
    }

    if (inviterId) loadFollowers();
    return () => { ignore = true; };
  }, [inviterId]);

  const memberIds = new Set((members || []).map((member) => member.user_id));
  const available = followers.filter((follower) => !memberIds.has(follower.id));

  return (
    <section className="invite-picker">
      <h3>Invite a follower</h3>
      {loading && <p>Loading followers...</p>}
      {error && <p role="alert">{error}</p>}
      {!loading && !error && available.length === 0 && (
        <p>No new followers can be invited.</p>
      )}
      {!loading && available.map((follower) => (
        <div className="invite-row" key={follower.id}>
          <span>
            {follower.first_name} {follower.last_name}
            {follower.nickname ? ` (${follower.nickname})` : ""}
          </span>
          <button type="button" onClick={() => onInvite(groupId, follower.id)}>
            Invite
          </button>
        </div>
      ))}
    </section>
  );
}
