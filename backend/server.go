// server.go is the SERVER part of "server / app / database":
// it opens the database (which runs the migrations), builds the router,
// serves uploaded images and listens on :8080. No business logic here.
package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"social-network/pkg/db/sqlite"
	"social-network/pkg/handlers"
)

func main() {
	// Paths are relative to backend/ locally and to /app in Docker,
	// so the same defaults work in both. Env vars let Docker override them.
	dbPath := getEnv("DB_PATH", "data/social.db")
	uploadsDir := getEnv("UPLOADS_DIR", "uploads")
	addr := ":" + getEnv("PORT", "8080")

	// SQLite creates the .db file but not its folder.
	for _, dir := range []string{filepath.Dir(dbPath), uploadsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatalf("cannot create %s: %v", dir, err)
		}
	}

	// DATABASE: C's sqlite.Open connects with foreign keys on
	// and applies every pending migration before we accept requests.
	db, err := sqlite.Open(dbPath)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	// TODO(B, Mon 30): hub := websocket.NewHub(); go hub.Run()
	// and pass it to RegisterRoutes.

	// APP: every API route is listed in pkg/handlers/routes.go.
	mux := http.NewServeMux()
	handlers.RegisterRoutes(mux, db)

	// Uploaded images: /uploads/<uuid>.png → uploads/<uuid>.png on disk.
	mux.Handle("GET /uploads/", serveUploads(uploadsDir))

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
		// Stops slow clients from holding connections open forever.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
		// No WriteTimeout on purpose: it would cut WebSocket
		// connections, which stay open for the whole session.
	}

	log.Printf("backend listening on %s (db: %s)", addr, dbPath)
	log.Fatal(srv.ListenAndServe())
}

// serveUploads serves files from dir, but:
//   - refuses directory listings, so nobody can browse every uploaded image;
//   - sets nosniff, so the browser trusts our Content-Type and never runs
//     an "image" as HTML or JS.
//
// http.Dir already blocks "../" path traversal.
func serveUploads(dir string) http.Handler {
	files := http.StripPrefix("/uploads/", http.FileServer(http.Dir(dir)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		files.ServeHTTP(w, r)
	})
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
