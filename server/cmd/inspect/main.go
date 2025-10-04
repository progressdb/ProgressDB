package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"progressdb/pkg/logger"
)

func main() {
	var p string
	flag.StringVar(&p, "path", "", "audit dir path to attach")
	flag.Parse()
	if p == "" {
		fmt.Fprintln(os.Stderr, "--path required")
		os.Exit(2)
	}
	logger.Init()
	fmt.Fprintf(os.Stdout, "calling AttachAuditFileSink(%s)\n", p)
	if err := logger.AttachAuditFileSink(p); err != nil {
		fmt.Fprintf(os.Stdout, "AttachAuditFileSink returned error: %v\n", err)
		os.Exit(1)
	}
	// print where audit.log would be
	f := filepath.Join(p, "audit.log")
	fmt.Fprintf(os.Stdout, "AttachAuditFileSink succeeded; audit file path: %s\n", f)
}
