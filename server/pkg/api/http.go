package api

import (
	"net/http"
	"progressdb/pkg/api/handlers"
	authpkg "progressdb/pkg/auth"

	"github.com/gorilla/mux"
)

// Handler constructs and returns the HTTP handler for the API by
// registering all handlers onto the mux.
func Handler() http.Handler {
	r := mux.NewRouter()

	// Compose subrouters for `/v1` and `/admin` and register handler groups directly here.
	api := r.PathPrefix("/v1").Subrouter()

	// Signing endpoint (backend keys only) — register on api root so it can be
	// called by backend SDKs that possess a backend API key.
	handlers.RegisterSigning(api)

	// Protected subrouter: endpoints that require a signed author identifier
	protected := api.NewRoute().Subrouter()
	protected.Use(authpkg.RequireSignedAuthor)
	handlers.RegisterMessages(protected)
	handlers.RegisterThreads(protected)

	admin := r.PathPrefix("/admin").Subrouter()
	handlers.RegisterAdmin(admin)

	return r
}
