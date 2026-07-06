// Package api provides the HTTP server for the Onshape Replay backend.
package api

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/abstractmelon/onshape-replay/internal/auth"
	"github.com/abstractmelon/onshape-replay/internal/ffmpeg"
	"github.com/abstractmelon/onshape-replay/internal/onshape"
	"github.com/abstractmelon/onshape-replay/internal/render"
	"github.com/abstractmelon/onshape-replay/internal/storage"
)

// Services groups all runtime dependencies passed into the router.
type Services struct {
	Sessions    *auth.Store
	OAuthHandler *auth.Handler
	Onshape     func(sess *auth.Session) *onshape.Client
	Queue       *render.Queue
	Encoder     *ffmpeg.Encoder
	StorageRoot string
	Log         *slog.Logger
	AllowedOrigins []string
}

// NewRouter wires all routes and returns the HTTP handler.
func NewRouter(svc Services) http.Handler {
	r := chi.NewRouter()

	// Base middleware.
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware(svc.AllowedOrigins))

	// Health.
	r.Get("/health", handleHealth)

	// OAuth flow (no auth required on these).
	r.Get("/auth/login", svc.OAuthHandler.LoginHandler)
	r.Get("/auth/callback", svc.OAuthHandler.CallbackHandler)
	r.Get("/auth/logout", svc.OAuthHandler.LogoutHandler)

	// Auth status check endpoint (called by the frontend on panel load).
	r.Get("/auth/status", func(w http.ResponseWriter, r *http.Request) {
		sess, ok := svc.Sessions.Get(r)
		if !ok || !auth.Authenticated(sess) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"status": "unauthenticated"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "authenticated"})
	})

	// Jobs (require authentication, no caching).
	r.With(requireAuth(svc.Sessions), noCache).Route("/jobs", func(r chi.Router) {
		r.Post("/", makeStartJobHandler(svc))
		r.Get("/current", makeCurrentJobHandler(svc))

		r.Route("/{jobId}", func(r chi.Router) {
			r.Get("/", makeGetJobHandler(svc))
			r.Get("/events", makeEventsHandler(svc))
			r.Post("/cancel", makeCancelHandler(svc))
			r.Get("/manifest", makeManifestHandler(svc))
			r.Get("/download/{format}", makeDownloadHandler(svc))
		})
	})

	// Static file serving for PNG frames (scoped to job download path above for
	// zip/mp4/gif; PNG sequence is served directly as frames).
	r.Handle("/storage/*", http.StripPrefix("/storage/", http.FileServer(http.Dir(svc.StorageRoot))))

	return r
}

// requireAuth is middleware that rejects unauthenticated requests with 401.
func requireAuth(sessions *auth.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess, ok := sessions.Get(r)
			if !ok || !auth.Authenticated(sess) {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// corsMiddleware adds CORS headers to all responses.
func corsMiddleware(origins []string) func(http.Handler) http.Handler {
	allowAll := len(origins) == 1 && origins[0] == "*"
	allowed := strings.Join(origins, ", ")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if allowAll {
				if origin != "" {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Vary", "Origin")
				} else {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				}
			} else if origin != "" {
				for _, o := range origins {
					if o == origin {
						w.Header().Set("Access-Control-Allow-Origin", origin)
						w.Header().Set("Vary", "Origin")
						break
					}
				}
			}
			_ = allowed

			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Credentials", "true")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// getSession extracts the session from the request. Panics if missing (should
// only be called in routes protected by requireAuth middleware).
func getSession(svc Services, r *http.Request) *auth.Session {
	sess, _ := svc.Sessions.Get(r)
	return sess
}

// onshapeClient builds an Onshape client for the authenticated session.
func onshapeClientForReq(svc Services, r *http.Request) *onshape.Client {
	sess := getSession(svc, r)
	return svc.Onshape(sess)
}

// storageLayout returns the storage paths for a job.
func storageLayout(svc Services, job *render.Job) storage.Paths {
	return storage.Layout(svc.StorageRoot, job.DocumentID, job.ElementID, job.ID)
}
