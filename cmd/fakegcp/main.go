package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/redscaresu/fakegcp/handlers"
	"github.com/redscaresu/fakegcp/repository"
)

func main() {
	port := flag.Int("port", 8080, "HTTP port")
	dbPath := flag.String("db", ":memory:", "SQLite path (:memory: for ephemeral)")
	echo := flag.Bool("echo", false, "echo mode: log all requests, return {\"ok\":true}")
	flag.Parse()

	if *echo {
		r := chi.NewRouter()
		r.Use(middleware.Logger)
		r.HandleFunc("/*", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"ok":true}`)
		})
		addr := fmt.Sprintf(":%d", *port)
		log.Printf("fakegcp echo mode on %s", addr)
		log.Fatal(http.ListenAndServe(addr, r))
	}

	// For :memory:, create a temp file so SQLite FK enforcement works with single connection
	actualPath := *dbPath
	if actualPath == ":memory:" {
		f, err := os.CreateTemp("", "fakegcp-*.db")
		if err != nil {
			log.Fatalf("create temp db: %v", err)
		}
		actualPath = f.Name()
		f.Close()
		defer os.Remove(actualPath)
	}

	repo, err := repository.New(actualPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}

	app := handlers.NewApplication(repo)
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	app.RegisterRoutes(r)

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("fakegcp on %s (db: %s)", addr, *dbPath)
	log.Fatal(http.ListenAndServe(addr, r))
}
