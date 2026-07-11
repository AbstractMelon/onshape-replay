package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/abstractmelon/onshape-replay/internal/api"
	"github.com/abstractmelon/onshape-replay/internal/auth"
	"github.com/abstractmelon/onshape-replay/internal/config"
	"github.com/abstractmelon/onshape-replay/internal/ffmpeg"
	"github.com/abstractmelon/onshape-replay/internal/onshape"
	"github.com/abstractmelon/onshape-replay/internal/render"
	"github.com/abstractmelon/onshape-replay/pkg/logging"
)

func main() {
	// Load .env file if present.
	err := godotenv.Load()
	if err != nil {
		logging.NewText().Error("failed to load .env file", "err", err)
	}

	log := logging.NewText()

	// Load configuration.
	cfg, err := config.Load()
	if err != nil {
		log.Error("configuration error", "err", err)
		os.Exit(1)
	}
	log.Info("oauth config loaded", "callback_url", cfg.OAuthCallbackURL, "client_id", cfg.OAuthClientID)

	// Verify FFmpeg is available before starting.
	encoder := ffmpeg.NewEncoder(cfg.FFmpegPath)
	checkCtx, checkCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer checkCancel()
	if err := encoder.CheckAvailable(checkCtx); err != nil {
		log.Error("FFmpeg check failed -- video/GIF encoding will not work", "err", err)
		// Not a fatal error: PNG sequence export still works without FFmpeg.
	}

	// Wire authentication.
	sessions := auth.NewStore()
	oauthHandler := auth.NewHandler(auth.OAuthConfig{
		ClientID:     cfg.OAuthClientID,
		ClientSecret: cfg.OAuthClientSecret,
		CallbackURL:  cfg.OAuthCallbackURL,
	}, sessions, log)

	// Onshape client factory: creates a client bound to the session's token.
	onshapeClientFactory := func(sess *auth.Session) *onshape.Client {
		return onshape.NewClient(log, func() string {
			return sess.AccessToken
		}, func(ctx context.Context) (string, error) {
			return oauthHandler.RefreshToken(ctx, sess, nil)
		})
	}

	queue := render.NewQueue()

	svc := api.Services{
		Sessions:       sessions,
		OAuthHandler:   oauthHandler,
		Onshape:        onshapeClientFactory,
		Queue:          queue,
		Encoder:        encoder,
		StorageRoot:    cfg.StorageRoot,
		Log:            log,
		AllowedOrigins: cfg.AllowedOrigins,
		FrontendFS:     frontendFS(),
	}

	router := api.NewRouter(svc)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // SSE streams are long-lived.
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Info("starting server", slog.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	<-stop
	log.Info("shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown error", "err", err)
	}
	log.Info("stopped")
}
