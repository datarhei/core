package server

import "net/http"

type Server interface {
	ServeHTTP(w http.ResponseWriter, r *http.Request)
	HTTPStatus() map[string]uint64
}
