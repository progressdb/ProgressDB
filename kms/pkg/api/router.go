package router

import (
	"net/http"

	handlers "github.com/progressdb/kms/pkg/api/routes"
	"github.com/progressdb/kms/pkg/store"
)

type Router struct {
	deps *handlers.Dependencies
	mux  *http.ServeMux
}

func New(provider handlers.ProviderInterface, store *store.Store) *Router {
	deps := &handlers.Dependencies{
		Provider: provider,
		Store:    store,
	}

	mux := http.NewServeMux()
	router := &Router{
		deps: deps,
		mux:  mux,
	}

	router.registerRoutes()

	return router
}

func (r *Router) Handler() http.Handler {
	return r.mux
}

func (r *Router) registerRoutes() {
	r.mux.HandleFunc("/healthz", r.deps.Health)
	r.mux.HandleFunc("/create_dek_for_thread", r.deps.CreateDEK)
	r.mux.HandleFunc("/get_wrapped", r.deps.GetWrapped)
	r.mux.HandleFunc("/encrypt", r.deps.Encrypt)
	r.mux.HandleFunc("/decrypt", r.deps.Decrypt)
	r.mux.HandleFunc("/rewrap", r.deps.Rewrap)
}
