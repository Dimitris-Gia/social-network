// Owner: C — browse + create

import { useEffect, useState } from "react";
import GroupCard from "../components/groups/GroupCard.jsx";
import { createGroup, joinGroup, listGroups } from "../api/groups.js";

export default function GroupsPage() {
  const [groups, setGroups] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");

  useEffect(() => {
    let ignore = false;

    async function loadGroups() {
      try {
        setLoading(true);
        setError("");
        const result = await listGroups();
        if (!ignore) {
          setGroups(result);
        }
      } catch (loadError) {
        if (!ignore) {
          setError(loadError.message);
        }
      } finally {
        if (!ignore) {
          setLoading(false);
        }
      }
    }

    loadGroups();

    return () => {
      ignore = true;
    };
  }, []);

  async function handleCreate(event) {
    event.preventDefault();

    try {
      setError("");
      const newGroup = await createGroup(title, description);
      setGroups((currentGroups) => [newGroup, ...currentGroups]);
      setTitle("");
      setDescription("");
    } catch (createError) {
      setError(createError.message);
    }
  }

  async function handleJoin(groupId) {
    try {
      setError("");
      await joinGroup(groupId);
      setGroups((currentGroups) =>
        currentGroups.map((group) =>
          group.id === groupId ? { ...group, status: "requested" } : group,
        ),
      );
    } catch (joinError) {
      setError(joinError.message);
    }
  }

  return (
    <section>
      <h1>Groups</h1>

      <form onSubmit={handleCreate}>
        <h2>Create group</h2>

        <label>
          Title
          <input
            type="text"
            value={title}
            onChange={(event) => setTitle(event.target.value)}
            required
          />
        </label>

        <label>
          Description
          <textarea
            value={description}
            onChange={(event) => setDescription(event.target.value)}
            required
          />
        </label>

        <button type="submit">Create</button>
      </form>

      {error && <p role="alert">{error}</p>}
      {loading && <p>Loading groups...</p>}

      {!loading && groups.length === 0 && <p>No groups yet.</p>}

      {!loading &&
        groups.map((group) => (
          <GroupCard
            key={group.id}
            group={group}
            onJoin={handleJoin}
          />
        ))}
    </section>
  );
}
