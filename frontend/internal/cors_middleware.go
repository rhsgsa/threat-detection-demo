package internal

import "net/http"

type CORSMiddleware struct {
	allowOrigin string
	nextHandler http.HandlerFunc
}

func InitCORSMiddleware(allowOrigin string, nextHandler http.HandlerFunc) CORSMiddleware {
	return CORSMiddleware{
		allowOrigin: allowOrigin,
		nextHandler: nextHandler,
	}
}

func (m CORSMiddleware) Handler(w http.ResponseWriter, r *http.Request) {
	if m.allowOrigin != "" {
		w.Header().Set("Access-Control-Allow-Origin", m.allowOrigin)
	}
	m.nextHandler(w, r)
}
