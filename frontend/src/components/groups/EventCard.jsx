// Owner: C — Going / Not going + counts
export default function EventCard({ event, onRespond }) {
  const date = new Date(event.event_time);
  const dateLabel = Number.isNaN(date.getTime())
    ? event.event_time
    : date.toLocaleString();

  return (
    <article className="group-event-card">
      <h3>{event.title}</h3>
      <p>{event.description}</p>
      <p><strong>When:</strong> {dateLabel}</p>
      <p>
        Going: {event.going_count} · Not going: {event.not_going_count}
      </p>
      <div className="event-actions">
        <button
          type="button"
          onClick={() => onRespond(event.id, "going")}
          aria-pressed={event.my_response === "going"}
        >
          Going
        </button>
        <button
          type="button"
          onClick={() => onRespond(event.id, "not_going")}
          aria-pressed={event.my_response === "not_going"}
        >
          Not going
        </button>
      </div>
    </article>
  );
}
