// Package api provides the HTTP server for the Onshape Replay backend.
package api

import (
	"io/fs"
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

// authCookieName is the name of the session cookie. Must match auth.cookieName.
const authCookieName = "oreplay_session"

// Services groups all runtime dependencies passed into the router.
type Services struct {
	Sessions       *auth.Store
	OAuthHandler   *auth.Handler
	Onshape        func(sess *auth.Session) *onshape.Client
	Queue          *render.Queue
	Encoder        *ffmpeg.Encoder
	StorageRoot    string
	Log            *slog.Logger
	AllowedOrigins []string
	FrontendFS     fs.FS
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
		_, err := r.Cookie(authCookieName)
		noCookie := err != nil

		sess, ok := svc.Sessions.Get(r)
		if !ok || !auth.Authenticated(sess) {
			switch {
			case noCookie:
				svc.Log.Debug("auth status: no session cookie", "remote", r.RemoteAddr)
			case !ok:
				svc.Log.Debug("auth status: session not found or expired", "remote", r.RemoteAddr)
			default:
				svc.Log.Debug("auth status: session has no access token", "remote", r.RemoteAddr)
			}
			writeJSON(w, http.StatusUnauthorized, map[string]string{"status": "unauthenticated"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "authenticated"})
	})

	// Named views (require authentication).
	r.With(requireAuth(svc.Sessions, svc.Log), noCache).Get("/named-views", makeNamedViewsHandler(svc))

	// Jobs (require authentication, no caching).
	r.With(requireAuth(svc.Sessions, svc.Log), noCache).Route("/jobs", func(r chi.Router) {
		r.Post("/", makeStartJobHandler(svc))
		r.Post("/preview", makePreviewHandler(svc))
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

	// Frontend SPA catch-all: serve embedded static files, fall back to
	// index.html for client-side routing.
	r.Group(func(r chi.Router) {
		r.Use(middleware.Compress(5))
		r.Get("/*", serveFrontend(svc.FrontendFS))
	})

	return r
}

// requireAuth is middleware that rejects unauthenticated requests with 401.
func requireAuth(sessions *auth.Store, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := r.Cookie(authCookieName)
			noCookie := err != nil

			sess, ok := sessions.Get(r)
			if !ok || !auth.Authenticated(sess) {
				switch {
				case noCookie:
					log.Warn("auth denied: no session cookie",
						"remote", r.RemoteAddr, "path", r.URL.Path,
					)
				case !ok:
					log.Warn("auth denied: session not found or expired",
						"remote", r.RemoteAddr, "path", r.URL.Path,
					)
				default:
					log.Warn("auth denied: session has no access token",
						"remote", r.RemoteAddr, "path", r.URL.Path,
					)
				}
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
	sess, ok := svc.Sessions.Get(r)
	if !ok {
		svc.Log.Warn("session not found in getSession. Caller should have checked auth")
	}
	return sess
}

// onshapeClient builds an Onshape client for the authenticated session.
func onshapeClientForReq(svc Services, r *http.Request) *onshape.Client {
	sess := getSession(svc, r)
	return svc.Onshape(sess)
}

// serveFrontend returns a handler that serves the embedded SPA frontend.
// It first tries to serve the exact path from the embedded filesystem; if the
// file is not found, it falls back to index.html for client-side routing.
func serveFrontend(frontend fs.FS) http.HandlerFunc {
	fileServer := http.FileServer(http.FS(frontend))

	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")

		// Check if the file exists in the embedded frontend.
		f, err := frontend.Open(path)
		if err != nil {
			// Serve the SPA fallback for client-side routing.
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}
		f.Close()

		fileServer.ServeHTTP(w, r)
	}
}

// storageLayout returns the storage paths for a job.
func storageLayout(svc Services, job *render.Job) storage.Paths {
	return storage.Layout(svc.StorageRoot, job.DocumentID, job.ElementID, job.ID)
}
