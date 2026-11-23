// Owner: A
// Entry point: open DB, run migrations, create hub, register routes,
// serve /uploads, listen on :8080.
package main

import (
	"log"
	"net/http"

	"social-network/pkg/db/sqlite"
)

const dbPath = "data/social.db"

func main() {
	db, err := sqlite.Open(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	log.Printf("database ready at %s", dbPath)

	// TODO (B): hub := websocket.NewHub(); go hub.Run()
	mux := http.NewServeMux()
	// TODO (A): handlers.RegisterRoutes(mux, db, hub)
	mux.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir("uploads"))))

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
