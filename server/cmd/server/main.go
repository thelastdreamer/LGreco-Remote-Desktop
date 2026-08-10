package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/lgreco/remote-desktop/server/internal/api"
	"github.com/lgreco/remote-desktop/server/internal/auth"
	"github.com/lgreco/remote-desktop/server/internal/config"
	"github.com/lgreco/remote-desktop/server/internal/db"
	"github.com/lgreco/remote-desktop/server/internal/signal"
)

func main() {
	cfg := config.Load()

	if err := db.Connect(cfg); err != nil {
		log.Printf("WARNING: database connection failed: %v", err)
		log.Println("running without database (in-memory mode)")
	} else {
		defer db.Close()
		if err := db.RunMigrations(); err != nil {
			log.Fatalf("migration failed: %v", err)
		}
	}

	auth.Init(cfg)

	hub := signal.NewHub()

	mux := http.NewServeMux()

	apiRouter := api.NewRouter(cfg)
	mux.Handle("/api/", apiRouter)
	mux.Handle("/api", apiRouter)

	mux.HandleFunc("/ws/signal", func(w http.ResponseWriter, r *http.Request) {
		hub.HandleWebSocket(w, r)
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("server starting on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
