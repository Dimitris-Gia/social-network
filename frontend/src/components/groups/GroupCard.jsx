// Owner: C — join button: Request / Requested / Member

import { Link } from "react-router-dom";

export default function GroupCard({ group, onJoin }) {
  const statusLabels = {
    member: "Member",
    invited: "Invited",
    requested: "Requested",
  };

  const hasStatusButton = group.status !== "none";

  return (
    <article>
      <h2>
        <Link to={`/groups/${group.id}`}>{group.title}</Link>
      </h2>

      <p>{group.description}</p>
      <p>{group.member_count} member(s)</p>

      {group.status === "none" ? (
        <button type="button" onClick={() => onJoin(group.id)}>
          Request to join
        </button>
      ) : hasStatusButton ? (
        <button type="button" disabled>
          {statusLabels[group.status]}
        </button>
      ) : null}
    </article>
  );
}
