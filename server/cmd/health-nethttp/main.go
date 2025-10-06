package main

import (
    "flag"
    "fmt"
    "net/http"
    "time"
)

func main() {
    addr := flag.String("addr", ":8082", "listen address for net/http health POC")
    ver := flag.String("version", "dev", "version string to return")
    flag.Parse()

    mux := http.NewServeMux()
    mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte("{\"status\":\"ok\",\"version\":\"" + *ver + "\"}"))
    })
    mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte("{\"status\":\"ok\",\"version\":\"" + *ver + "\"}"))
    })

    srv := &http.Server{
        Addr:         *addr,
        Handler:      mux,
        ReadTimeout:  5 * time.Second,
        WriteTimeout: 5 * time.Second,
    }
    fmt.Printf("net/http health POC listening on %s\n", *addr)
    if err := srv.ListenAndServe(); err != nil {
        fmt.Printf("net/http server exit: %v\n", err)
    }
}

