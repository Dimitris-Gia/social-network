# API contract

One section per owner. For each endpoint: method, path, request JSON, response JSON, error codes.

## A: Accounts & social graph

TODO

## B: Posts & real-time

TODO

## C: Groups & events

Όλες οι διαδρομές απαιτούν σύνδεση, εκτός αν αναφέρεται διαφορετικά. Οι ταυτότητες προέρχονται από το session cookie.

### Δημιουργία και λίστες ομάδων

- `POST /api/groups`
  - Request: `{"title":"...","description":"..."}`
  - Response: `201` + το αντικείμενο της ομάδας
  - Σφάλματα: `400` μη έγκυρα πεδία, `401` μη συνδεδεμένος χρήστης
- `GET /api/groups`
  - Response: `200` + λίστα όλων των ομάδων και της κατάστασης του τρέχοντος χρήστη (`member`, `invited`, `requested`, `none`)
- `GET /api/groups/{id}`
  - Response: `200` + ομάδα, δημιουργός, μέλη και κατάσταση χρήστη
  - Σφάλματα: `404` η ομάδα δεν υπάρχει

### Αιτήματα συμμετοχής και προσκλήσεις

- `POST /api/groups/{id}/join`
  - Request: `{}` ή χωρίς body
  - Response: `201` + `{}` μετά την εγγραφή κατάστασης `requested`
  - Σφάλματα: `409` υπάρχει ήδη αιτήματος/πρόσκληση/μέλος
- `POST /api/groups/{id}/invite`
  - Request: `{"user_id":2}`
  - Response: `201` + κατάσταση `invited`
  - Σφάλματα: `403` καλών χρήστης δεν είναι μέλος, `409` χρήστης ήδη μέλος/προσκεκλημένος
  - Όποιο μέλος μπορεί να προσκαλέσει έναν από τους δικούς του followers
- `POST /api/groups/{id}/members/{user_id}/accept`
  - Request: `{}`
  - Response: `200` + `{}` με κατάσταση `member`
  - Για προσκλήσεις: αποδέχεται ο προσκεκλημένος.
  - Για join request: αποδέχεται μόνο ο creator της ομάδας.
- `POST /api/groups/{id}/members/{user_id}/decline`
  - Request: `{}`
  - Response: `204`
  - Για προσκλήσεις: αρνείται ο προσκεκλημένος.
  - Για join request: αρνείται μόνο ο creator της ομάδας.
- `GET /api/groups/{id}/members`
  - Response: `200` + μόνο επιβεβαιωμένα μέλη
  - Σφάλματα: `403` ο requester δεν είναι μέλος

### Events

- `POST /api/groups/{id}/events`
  - Request: `{"title":"...","description":"...","event_time":"2026-12-01T18:30"}`
  - Response: `201` + το event
  - Μόνο μέλος μπορεί να δημιουργήσει event.
- `GET /api/groups/{id}/events`
  - Response: `200` + λίστα events με counts `going`, `not_going` και την απάντηση του τρέχοντος χρήστη
  - Σφάλματα: `403` ο requester δεν είναι μέλος
- `POST /api/groups/{id}/events/{event_id}/response`
  - Request: `{"response":"going"}` ή `{"response":"not_going"}`
  - Response: `200` + ενημερωμένα counts
  - Μόνο μέλος μπορεί να απαντήσει. Η ίδια απάντηση μπορεί να αλλάξει.

Κοινά σφάλματα: `401 Unauthorized`, `403 Forbidden`, `404 Not Found`, `409 Conflict`, `500 Internal Server Error`.
