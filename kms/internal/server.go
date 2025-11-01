package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"

	handlers "github.com/progressdb/kms/pkg/api/routes"
	security "github.com/progressdb/kms/pkg/core"
	"github.com/progressdb/kms/pkg/store"
)

type Server struct {
	server *http.Server
	store  *store.Store
}

func New(endpoint string, provider security.KMSProvider, dataDir string) (*Server, error) {
	st, err := store.New(dataDir + "/kms.db")
	if err != nil {
		return nil, fmt.Errorf("failed to open store: %w", err)
	}

	deps := &handlers.Dependencies{
		Provider: provider,
		Store:    st,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", deps.Health)
	mux.HandleFunc("/create_dek_for_thread", deps.CreateDEK)
	mux.HandleFunc("/get_wrapped", deps.GetWrapped)
	mux.HandleFunc("/encrypt", deps.Encrypt)
	mux.HandleFunc("/decrypt", deps.Decrypt)
	mux.HandleFunc("/rewrap", deps.Rewrap)

	return &Server{
		server: &http.Server{
			Handler: mux,
		},
		store: st,
	}, nil
}

func (s *Server) Start(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen tcp %s: %w", addr, err)
	}
	log.Printf("listening on %s", addr)

	return s.server.Serve(ln)
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *Server) Close() error {
	return s.store.Close()
}
