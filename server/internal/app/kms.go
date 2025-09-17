package app

import (
    "context"
    "encoding/hex"
    "fmt"
    "log"
    "net"
    "os"
    "os/exec"
    "path/filepath"
    "strings"

    "progressdb/pkg/kms"
    "progressdb/pkg/security"
)

// setupKMS starts and registers KMS when encryption is enabled.
func (a *App) setupKMS(ctx context.Context) error {
	socket := os.Getenv("PROGRESSDB_KMS_SOCKET")
	if socket == "" {
		socket = "/tmp/progressdb-kms.sock"
	}
	dataDir := os.Getenv("PROGRESSDB_KMS_DATA_DIR")
	if dataDir == "" {
		dataDir = "./kms-data"
	}
    bin := os.Getenv("PROGRESSDB_KMS_BINARY")
    if bin == "" {
        // Prefer an installed `kms` binary on PATH (e.g. installed via
        // `go install github.com/progressdb/kms/cmd/kms@latest`). This makes
        // runtime deployment easier: operators can install the KMS into
        // $GOBIN or /usr/local/bin and the server will spawn that binary.
        if p, err := exec.LookPath("kms"); err == nil {
            bin = p
        } else {
            // Fallback for local development: look for a sibling `kms`
            exePath, err := os.Executable()
            if err != nil {
                return fmt.Errorf("failed to determine executable path: %w", err)
            }
            bin = filepath.Join(filepath.Dir(exePath), "kms")
        }
    }

	useEnc := a.eff.Config.Security.Encryption.Use
	if ev := strings.TrimSpace(os.Getenv("PROGRESSDB_USE_ENCRYPTION")); ev != "" {
		switch strings.ToLower(ev) {
		case "1", "true", "yes":
			useEnc = true
		default:
			useEnc = false
		}
	}

	if !useEnc {
		log.Printf("encryption enabled: false")
		return nil
	}

	// master key selection
	var mk string
	switch {
	case strings.TrimSpace(a.eff.Config.Security.KMS.MasterKeyFile) != "":
		mkFile := strings.TrimSpace(a.eff.Config.Security.KMS.MasterKeyFile)
		keyb, err := os.ReadFile(mkFile)
		if err != nil {
			return fmt.Errorf("failed to read master key file %s: %w", mkFile, err)
		}
		mk = strings.TrimSpace(string(keyb))
	case strings.TrimSpace(a.eff.Config.Security.KMS.MasterKeyHex) != "":
		mk = strings.TrimSpace(a.eff.Config.Security.KMS.MasterKeyHex)
	default:
		return fmt.Errorf("PROGRESSDB_USE_ENCRYPTION=true but no master key provided in server config. Set security.kms.master_key_file or security.kms.master_key_hex")
	}
	if mk == "" {
		return fmt.Errorf("master key is empty")
	}
	if kb, err := hex.DecodeString(mk); err != nil || len(kb) != 32 {
		return fmt.Errorf("invalid master_key_hex: must be 64-hex (32 bytes)")
	}

	// write launcher config
	lcfg := &kms.LauncherConfig{MasterKeyHex: mk, Socket: socket, DataDir: dataDir}
	kmsCfgPath, err := kms.CreateSecureConfigFile(lcfg, dataDir)
	if err != nil {
		return fmt.Errorf("failed to write kms config: %w", err)
	}

	// prebind socket
	var (
		parentListenerClose func()
		ln                  *net.UnixListener
	)
	if socket != "" {
		if dir := filepath.Dir(socket); dir != "" {
			_ = os.MkdirAll(dir, 0700)
		}
		if l, err := net.Listen("unix", socket); err == nil {
			if ul, ok := l.(*net.UnixListener); ok {
				ln = ul
				if f, ferr := ul.File(); ferr == nil {
					parentListenerClose = func() {
						_ = ul.Close()
						_ = f.Close()
					}
				} else {
					_ = ul.Close()
				}
			} else {
				_ = l.Close()
			}
		}
	}

	h, err := kms.StartChildLauncher(ctx, bin, kmsCfgPath, ln)
	if parentListenerClose != nil {
		parentListenerClose()
	}
	if err != nil {
		return fmt.Errorf("failed to start KMS: %w", err)
	}
	a.child = &kms.CmdHandle{Cmd: h.Cmd}
	a.rc = kms.NewRemoteClient(socket)
	security.RegisterKMSProvider(a.rc)
	if err := a.rc.Health(); err != nil {
		return fmt.Errorf("KMS health check failed at %s: %w; ensure KMS is installed and reachable", socket, err)
	}

	kctx, cancel := context.WithCancel(ctx)
	a.cancel = cancel
	go func() { <-kctx.Done() }()
	log.Printf("encryption enabled: true (KMS socket=%s)", socket)
	_ = kmsCfgPath // keep variable if future use
	return nil
}
