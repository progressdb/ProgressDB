package handlers

import (
	"net/http"

	"github.com/progressdb/kms/pkg/api"
)

func (d *Dependencies) Health(w http.ResponseWriter, r *http.Request) {
	if d.Provider != nil {
		if err := d.Provider.Health(); err != nil {
			api.WriteServiceUnavailable(w, "service unhealthy")
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
