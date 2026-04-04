package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"

	"github.com/divinedev111/airbag/internal/api"
	"github.com/divinedev111/airbag/internal/ingest"
	"github.com/divinedev111/airbag/internal/store"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	dbPath := flag.String("db", "airbag.db", "SQLite database path")
	flag.Parse()

	db, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	mux := http.NewServeMux()

	// Ingest endpoint (from client SDKs)
	ingestHandler := ingest.New(db)
	mux.Handle("POST /api/ingest/{project}", ingestHandler)

	// Dashboard API
	apiHandler := api.New(db)
	apiRoutes := apiHandler.Routes()
	mux.Handle("/api/projects/", apiRoutes)
	mux.Handle("/api/issues/", apiRoutes)

	srv := &http.Server{Addr: *addr, Handler: mux}

	go func() {
		log.Printf("airbag listening on %s", *addr)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit

	fmt.Println("\nshutting down...")
	srv.Close()
}
