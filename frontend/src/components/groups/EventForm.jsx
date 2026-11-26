import { useState } from "react";

export default function EventForm({ onCreate }) {
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [eventTime, setEventTime] = useState("");
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(event) {
    event.preventDefault();

    try {
      setSubmitting(true);
      await onCreate({ title, description, eventTime });
      setTitle("");
      setDescription("");
      setEventTime("");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form className="group-event-form" onSubmit={handleSubmit}>
      <h3>Create event</h3>
      <label>
        Title
        <input value={title} onChange={(e) => setTitle(e.target.value)} required />
      </label>
      <label>
        Description
        <textarea
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          required
        />
      </label>
      <label>
        Date and time
        <input
          type="datetime-local"
          value={eventTime}
          onChange={(e) => setEventTime(e.target.value)}
          required
        />
      </label>
      <button type="submit" disabled={submitting}>Create event</button>
    </form>
  );
}
