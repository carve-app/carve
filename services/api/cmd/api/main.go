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

	"github.com/carve-app/carve/services/api/internal/auth"
	"github.com/carve-app/carve/services/api/internal/cards"
	"github.com/carve-app/carve/services/api/internal/db"
	"github.com/carve-app/carve/services/api/internal/immersion"
	"github.com/carve-app/carve/services/api/internal/nlp"
	"github.com/carve-app/carve/services/api/internal/review"
	"github.com/carve-app/carve/services/api/internal/users"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(log)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := db.Connect(ctx)
	if err != nil {
		slog.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	slog.Info("database connected")

	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// Health
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"status":"ok","service":"api"}`)
	})

	// Auth routes (no auth middleware)
	authHandler := auth.NewHandler(pool)
	r.Route("/v1/auth", func(r chi.Router) {
		r.Post("/register", authHandler.Register)
		r.Post("/login", authHandler.Login)
		r.Post("/refresh", authHandler.Refresh)
		r.Post("/logout", authHandler.Logout)
	})

	// Protected routes
	userHandler := users.NewHandler(pool)
	cardsHandler := cards.NewHandler(pool)
	reviewHandler := review.NewHandler(pool)
	immersionHandler := immersion.NewHandler(pool)
	nlpProxy := nlp.NewProxy()

	r.Route("/v1", func(r chi.Router) {
		r.Use(auth.Middleware)

		// Users
		r.Get("/users/me", userHandler.Me)
		r.Patch("/users/me", userHandler.Update)

		// Cards
		r.Post("/cards", cardsHandler.Create)
		r.Get("/cards", cardsHandler.List)
		r.Delete("/cards/{id}", cardsHandler.Delete)

		// Review
		r.Get("/review/due-count", reviewHandler.DueCount)

		// Immersion
		r.Post("/immersion", immersionHandler.Create)

		// NLP proxy
		r.Post("/nlp/tokenize", nlpProxy.Tokenize)
		r.Post("/nlp/lookup", nlpProxy.Lookup)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port

	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	slog.Info("starting server", "addr", addr)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down gracefully")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	srv.Shutdown(shutCtx)
}
