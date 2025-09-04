package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"progressdb/pkg/api"
	"progressdb/pkg/store"
)

const banner = `
██████╗ ██████╗  ██████╗  ██████╗ ██████╗ ███████╗███████╗███████╗    ██████╗ ██████╗ 
██╔══██╗██╔══██╗██╔═══██╗██╔════╝ ██╔══██╗██╔════╝██╔════╝██╔════╝    ██╔══██╗██╔══██╗
██████╔╝██████╔╝██║   ██║██║  ███╗██████╔╝█████╗  ███████╗███████╗    ██║  ██║██████╔╝
██╔═══╝ ██╔══██╗██║   ██║██║   ██║██╔══██╗██╔══╝  ╚════██║╚════██║    ██║  ██║██╔══██╗
██║     ██║  ██║╚██████╔╝╚██████╔╝██║  ██║███████╗███████║███████║    ██████╔╝██████╔╝
╚═╝     ╚═╝  ╚═╝ ╚═════╝  ╚═════╝ ╚═╝  ╚═╝╚══════╝╚══════╝╚══════╝    ╚═════╝ ╚═════╝ 
                                                                                    
`

func printHelper(addr, dbPath string) {
	fmt.Println(banner)
	fmt.Println("== Config =====================================================")
	fmt.Printf("Listen:   %s\n", addr)
	fmt.Printf("DB Path:  %s\n", dbPath)
	fmt.Println("\n== Endpoints ==================================================")
	fmt.Println("GET  /?thread=<id>&msg=<text>  - Append a message and list messages")
	fmt.Println("GET  /?thread=<id>              - List messages in a thread")
	fmt.Println("POST / (form: thread,msg)       - Append a message and list messages")
	fmt.Println("\n== Examples ===================================================")
	fmt.Printf("curl 'http://localhost%s/?thread=t1&msg=hello'\n", addr)
	fmt.Printf("curl 'http://localhost%s/?thread=t1'\n", addr)
	fmt.Println("\n== Production? =================================================")
	fmt.Println("Set a proper storage path (--db)")
	fmt.Println("Add API key or auth for production")
}

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	dbPath := flag.String("db", "./.database", "Pebble DB path")
	flag.Parse()

	if err := store.Open(*dbPath); err != nil {
		log.Fatalf("failed to open pebble at %s: %v", *dbPath, err)
	}

	printHelper(*addr, *dbPath)

	mux := http.NewServeMux()
	mux.Handle("/", api.Handler())

	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}
