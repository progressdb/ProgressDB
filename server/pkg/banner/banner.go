package banner

import (
	"fmt"
)

const banner = `
██████╗ ██████╗  ██████╗  ██████╗ ██████╗ ███████╗███████╗███████╗    ██████╗ ██████╗ 
██╔══██╗██╔══██╗██╔═══██╗██╔════╝ ██╔══██╗██╔════╝██╔════╝██╔════╝    ██╔══██╗██╔══██╗
██████╔╝██████╔╝██║   ██║██║  ███╗██████╔╝█████╗  ███████╗███████╗    ██║  ██║██████╔╝
██╔═══╝ ██╔══██╗██║   ██║██║   ██║██╔══██╗██╔══╝  ╚════██║╚════██║    ██║  ██║██╔══██╗
██║     ██║  ██║╚██████╔╝╚██████╔╝██║  ██║███████╗███████║███████║    ██████╔╝██████╔╝
╚═╝     ╚═╝  ╚═╝ ╚═════╝  ╚═════╝ ╚═╝  ╚═╝╚══════╝╚══════╝╚══════╝    ╚═════╝ ╚═════╝                                                                                 
`

func Print(addr, dbPath, sources, version string) {
	fmt.Print(banner)
	fmt.Println("== Config =====================================================")
	fmt.Printf("Listen:   %s\n", addr)
	fmt.Printf("DB Path:  %s\n", dbPath)
	if version != "" {
		fmt.Printf("Version:  %s\n", version)
	}
	if sources != "" {
		fmt.Printf("Config sources: %s\n", sources)
	}
	fmt.Println("\n== Endpoints ==================================================")
	fmt.Println("POST /v1/messages - Add a message (JSON: id, thread, author, ts, body)")
	fmt.Println("GET  /v1/messages?thread=<id>&limit=<n> - List messages in a thread (JSON response)")
	fmt.Println("\n== Examples ===================================================")
	fmt.Printf("curl -X POST 'http://localhost%s/v1/messages' -d '{body: {text: \"hello\"}}'\n", addr)
	fmt.Printf("curl 'http://localhost%s/v1/messages?thread=t1&limit=10'\n", addr)
	fmt.Println("\n== Production? =================================================")
	fmt.Println("Set a proper storage path (--db)")
	fmt.Println("Add API key or authentication for production use")
}
