package api

import (
	"github.com/valyala/fasthttp"
	"progressdb/pkg/router"

	"progressdb/pkg/api/handlers"
)

// Handler returns the fasthttp handler for the ProgressDB API.
func Handler() fasthttp.RequestHandler {
	r := router.New()
	handlers.RegisterSigningFast(r)
	handlers.RegisterMessagesFast(r)
	handlers.RegisterThreadsFast(r)
	handlers.RegisterAdminFast(r)
	return r.Handler
}
