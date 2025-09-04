package main

import (
    "flag"
    "log"
    "net/http"

    "progressdb/pkg/api"
    "progressdb/pkg/store"
)

func main() {
    addr := flag.String("addr", ":8080", "HTTP listen address")
    dbPath := flag.String("db", "./data", "Pebble DB path")
    flag.Parse()

    if err := store.Open(*dbPath); err != nil {
        log.Fatalf("failed to open pebble at %s: %v", *dbPath, err)
    }

    mux := http.NewServeMux()
    mux.Handle("/", api.Handler())

    log.Printf("progressdb listening on %s (db: %s)", *addr, *dbPath)
    if err := http.ListenAndServe(*addr, mux); err != nil {
        log.Fatal(err)
    }
}

