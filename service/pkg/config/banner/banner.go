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

	fmt.Println("\n== Production? =================================================")
	// API keys
	be := 0
	fe := 0
	ak := 0
	if eff.Config != nil {
		be = len(eff.Config.Server.APIKeys.Backend)
		fe = len(eff.Config.Server.APIKeys.Frontend)
		ak = len(eff.Config.Server.APIKeys.Admin)
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

	// DB path
	if eff.DBPath != "" {
		fmt.Printf("- DB Path: %s\n", eff.DBPath)
	} else {
		fmt.Println("- DB Path: not set (use --db or PROGRESSDB_SERVER_DB_PATH)")
	}

	// Encryption / KMS
	enc := false
	if eff.Config != nil && eff.Config.Encryption.Enabled {
		enc = true
	}
	if enc {
		// check for master key or external endpoint
		hasMaster := false
		hasEndpoint := false
		if eff.Config != nil {
			if strings.TrimSpace(eff.Config.Encryption.KMS.MasterKeyFile) != "" || strings.TrimSpace(eff.Config.Encryption.KMS.MasterKeyHex) != "" {
				hasMaster = true
			}
			if strings.TrimSpace(eff.Config.Encryption.KMS.Endpoint) != "" {
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

	fmt.Println("\nFor configuration guidance, visit: https://progressdb.dev/docs")
}
