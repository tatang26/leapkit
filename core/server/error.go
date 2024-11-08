package server

import (
	"log/slog"
	"net/http"
)

// Error logs the error and sends an internal server error response.
func Error(w http.ResponseWriter, err error, HTTPStatus int) {
	slog.Error(err.Error())

	http.Error(w, err.Error(), HTTPStatus)
}

type errorHandlerFn func(w http.ResponseWriter, r *http.Request, err error)

var (
	errorHandlerMap = map[int]errorHandlerFn{
		http.StatusNotFound: func(w http.ResponseWriter, r *http.Request, err error) {
			w.Write([]byte("404 page not found"))
		},

		http.StatusInternalServerError: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Error(err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
		},
	}
)
