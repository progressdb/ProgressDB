package api

import (
	"net/http"
	"progressdb/pkg/api/handlers"
	authpkg "progressdb/pkg/auth"

	"github.com/gorilla/mux"
)

// Handler returns the main HTTP handler for the API, registering all endpoints.
func Handler() http.Handler {
	r := mux.NewRouter()

	// Set up subrouters for versioned API and admin endpoints.
	api := r.PathPrefix("/v1").Subrouter()

	// Register the signing endpoint on the API root.
	// This is used by backend SDKs with a backend API key.
	handlers.RegisterSigning(api)

	// Register endpoints that require a signed author identifier.
	protected := api.NewRoute().Subrouter()
	protected.Use(authpkg.RequireSignedAuthor)
	handlers.RegisterMessages(protected)
	handlers.RegisterThreads(protected)

	// Register admin endpoints.
	admin := r.PathPrefix("/admin").Subrouter()
	handlers.RegisterAdmin(admin)

	return r
}
