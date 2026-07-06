package api

import "net/http"

// middleware.go contains shared middleware functions used by the router.
// CORS and requireAuth are both defined here to keep router.go clean.
// (Additional middleware can be added here as needed.)

// noCache sets headers to prevent caching of API responses.
func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		next.ServeHTTP(w, r)
	})
}
