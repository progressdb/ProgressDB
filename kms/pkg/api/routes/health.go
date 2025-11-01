package handlers

import (
	"net/http"
)

func (d *Dependencies) Health(w http.ResponseWriter, r *http.Request) {
	if d.Provider != nil {
		if err := d.Provider.Health(); err != nil {
			http.Error(w, "service unhealthy", http.StatusServiceUnavailable)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
