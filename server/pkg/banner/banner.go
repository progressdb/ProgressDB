package banner

import (
	"fmt"
	"strings"

	"progressdb/pkg/config"
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
	// Deprecated: previous signature printed explicit fields. Newer callers
	// pass an effective config so we can display runtime info (encryption,
	// config sources) centrally.
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

// PrintWithEff prints the banner using an EffectiveConfigResult which
// provides richer context (config, addr, dbpath, source).
func PrintWithEff(eff config.EffectiveConfigResult, version string) {
	var addr = eff.Addr
	if addr == "" && eff.Config != nil {
		addr = eff.Config.Addr()
	}
	var dbPath = eff.DBPath
	if dbPath == "" && eff.Config != nil {
		dbPath = eff.Config.Server.DBPath
	}
	var src = eff.Source
	if src == "" {
		src = "flags"
	}

	fmt.Print(banner)
	fmt.Println("== Config =====================================================")
	fmt.Printf("Listen:   %s\n", addr)
	fmt.Printf("DB Path:  %s\n", dbPath)
	if version != "" {
		fmt.Printf("Version:  %s\n", version)
	}
	fmt.Printf("Config: %s\n", src)

	fmt.Println("\n== Examples ===================================================")
	fmt.Println("curl -X POST 'http://<host>:<port>/v1/messages' -d '{body: {text: \"hello\"}}'")
	fmt.Println("curl 'http://<host>:<port>/v1/messages?thread=t1&limit=10'")
	fmt.Println("\n== Production? =================================================")
	// fmt.Println("Quick checks (recommended before production):")
	// API keys
	be := 0
	fe := 0
	ak := 0
	if eff.Config != nil {
		be = len(eff.Config.Security.APIKeys.Backend)
		fe = len(eff.Config.Security.APIKeys.Frontend)
		ak = len(eff.Config.Security.APIKeys.Admin)
	}
	if be > 0 {
		fmt.Printf("- Backend API keys: OK (%d)\n", be)
	} else {
		fmt.Println("- Backend API keys: MISSING (required for backend services)")
	}
	if fe > 0 {
		fmt.Printf("- Frontend API keys: OK (%d)\n", fe)
	} else {
		fmt.Println("- Frontend API keys: MISSING (required for client access)")
	}
	if ak > 0 {
		fmt.Printf("- Admin API keys: OK (%d)\n", ak)
	} else {
		fmt.Println("- Admin API keys: MISSING (required for admin tooling)")
	}

	// TLS
	tlsOK := false
	if eff.Config != nil && eff.Config.Server.TLS.CertFile != "" && eff.Config.Server.TLS.KeyFile != "" {
		tlsOK = true
	}
	if tlsOK {
		fmt.Println("- TLS: configured")
	} else {
		fmt.Println("- TLS: unconfigured")
	}

	// DB path
	if eff.DBPath != "" {
		fmt.Printf("- DB Path: %s\n", eff.DBPath)
	} else {
		fmt.Println("- DB Path: not set (use --db or PROGRESSDB_SERVER_DB_PATH)")
	}

	// Encryption / KMS
	enc := false
	if eff.Config != nil && eff.Config.Security.Encryption.Use {
		enc = true
	}
	if enc {
		// check for master key or external endpoint
		hasMaster := false
		hasEndpoint := false
		if eff.Config != nil {
			if strings.TrimSpace(eff.Config.Security.KMS.MasterKeyFile) != "" || strings.TrimSpace(eff.Config.Security.KMS.MasterKeyHex) != "" {
				hasMaster = true
			}
			if strings.TrimSpace(eff.Config.Security.KMS.Endpoint) != "" {
				hasEndpoint = true
			}
		}
		if hasMaster {
			fmt.Println("- Encryption: enabled (embedded)")
		} else if hasEndpoint {
			fmt.Println("- Encryption: enabled (external)")
		} else {
			fmt.Println("- Encryption: enabled (unconfigured)")
		}
	} else {
		fmt.Println("- Encryption: disabled")
	}

	// Retention
	retEnabled := false
	retInfo := ""
	if eff.Config != nil {
		retEnabled = eff.Config.Retention.Enabled
		if retEnabled {
			if eff.Config.Retention.Cron != "" {
				retInfo = "cron=" + eff.Config.Retention.Cron
			} else if eff.Config.Retention.Period != "" {
				retInfo = "period=" + eff.Config.Retention.Period
			}
		}
	}
	if retEnabled {
		if retInfo != "" {
			fmt.Printf("- Retention: enabled (%s)\n", retInfo)
		} else {
			fmt.Println("- Retention: enabled")
		}
	} else {
		fmt.Println("- Retention: disabled")
	}

	fmt.Println("\nRead the config docs to set up encryption and KMS: server/docs/encryption.md and docs/configs/README.md")

	fmt.Println("\n== Logs: =================================================")
}
