package api

import (
	"net/http"
	"progressdb/pkg/api/handlers"

	"github.com/gorilla/mux"
)

// Handler constructs and returns the HTTP handler for the API by
// registering all handlers onto the mux.
func Handler() http.Handler {
	r := mux.NewRouter()

	// Compose subrouters for `/v1` and `/admin` and register handler groups directly here.
	api := r.PathPrefix("/v1").Subrouter()
	handlers.RegisterMessages(api)
	handlers.RegisterThreads(api)

	admin := r.PathPrefix("/admin").Subrouter()
	handlers.RegisterAdmin(admin)

	return r
}
