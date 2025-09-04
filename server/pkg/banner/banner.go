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

func Print(addr, dbPath string) {
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
