# Social Network

Owner: A — fill in during week 3.

## Run in development

```bash
# backend (from backend/)
go run .

# frontend (from frontend/)
npm install
npm run dev
```

## Run with Docker

```bash
./build.sh
```

## Database (for auditors)

Inside the backend container the database lives at `/app/data/social.db`:

```bash
docker exec -it backend sqlite3 /app/data/social.db .tables
```

See [docs/folder-structure.md](docs/folder-structure.md) for the project layout.
