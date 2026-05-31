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
	"github.com/carve-app/carve/services/api/internal/billing"
	"github.com/carve-app/carve/services/api/internal/cards"
	"github.com/carve-app/carve/services/api/internal/db"
	"github.com/carve-app/carve/services/api/internal/decks"
	"github.com/carve-app/carve/services/api/internal/export"
	"github.com/carve-app/carve/services/api/internal/immersion"
	"github.com/carve-app/carve/services/api/internal/importer"
	"github.com/carve-app/carve/services/api/internal/library"
	"github.com/carve-app/carve/services/api/internal/nlp"
	"github.com/carve-app/carve/services/api/internal/onboarding"
	"github.com/carve-app/carve/services/api/internal/output"
	"github.com/carve-app/carve/services/api/internal/review"
	"github.com/carve-app/carve/services/api/internal/settings"
	"github.com/carve-app/carve/services/api/internal/stats"
	"github.com/carve-app/carve/services/api/internal/users"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
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
	r.Use(chimw.RealIP)
	r.Use(chimw.RequestID)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(30 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:*", "https://localhost:*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

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
	decksHandler := decks.NewHandler(pool)
	exportHandler := export.NewHandler(pool)
	settingsHandler := settings.NewHandler(pool)
	billingHandler := billing.NewHandler(pool)
	statsHandler := stats.NewHandler(pool)
	libraryHandler := library.NewHandler(pool)
	importerHandler := importer.NewHandler(pool)
	outputHandler := output.NewHandler(pool)
	onboardingHandler := onboarding.NewHandler(pool)

	r.Route("/v1", func(r chi.Router) {
		r.Use(auth.Middleware)

		// Users
		r.Get("/users/me", userHandler.Me)
		r.Patch("/users/me", userHandler.Update)
		r.Delete("/users/me", userHandler.Delete)

		// Onboarding
		r.Post("/onboarding/known-words", onboardingHandler.KnownWords)
		r.Post("/onboarding/starter-deck", onboardingHandler.StarterDeck)

		// Cards
		r.Post("/cards", cardsHandler.Create)
		r.Get("/cards", cardsHandler.List)
		r.Get("/cards/{id}", cardsHandler.Get)
		r.Delete("/cards/{id}", cardsHandler.Delete)
		r.Post("/cards/{id}/media", cardsHandler.AttachMedia)

		// Review
		r.Get("/review/due-count", reviewHandler.DueCount)
		r.Get("/review/session", reviewHandler.Session)
		r.Post("/review/events", reviewHandler.SubmitEvent)
		r.Get("/review/intervals", reviewHandler.Intervals)
		r.Get("/review/forecast", reviewHandler.Forecast)
		r.Get("/review/notifications", reviewHandler.Notifications)
		r.Post("/review/notifications/{id}/read", reviewHandler.MarkNotificationRead)

		// Decks
		r.Get("/decks", decksHandler.List)
		r.Post("/decks", decksHandler.Create)
		r.Patch("/decks/{id}", decksHandler.Update)
		r.Delete("/decks/{id}", decksHandler.DeleteDeck)
		r.Post("/decks/{id}/subscribe", decksHandler.Subscribe)
		r.Delete("/decks/{id}/subscribe", decksHandler.Unsubscribe)
		r.Post("/decks/{id}/rate", decksHandler.Rate)

		// Export
		r.Get("/export", exportHandler.Export)

		// Settings
		r.Get("/settings/fsrs", settingsHandler.GetFSRS)
		r.Put("/settings/fsrs", settingsHandler.PutFSRS)
		r.Get("/settings/workload-preview", settingsHandler.WorkloadPreview)

		// Immersion
		r.Post("/immersion", immersionHandler.Create)

		// Billing
		r.Get("/billing/subscription", billingHandler.Subscription)
		r.Post("/billing/checkout", billingHandler.Checkout)

		// Stats dashboard
		r.Get("/stats", statsHandler.Dashboard)

		// Library
		r.Get("/library", libraryHandler.List)
		r.Post("/library", libraryHandler.Add)
		r.Get("/library/{id}/reader", libraryHandler.Read)
		r.Post("/library/import", libraryHandler.ImportFile)
		r.Delete("/library/{id}", libraryHandler.Delete)

		// Import
		r.Post("/import/anki", importerHandler.ImportAnki)
		r.Post("/import/migaku-csv", importerHandler.ImportMigakuCSV)
		r.Post("/import/yomitan", importerHandler.ImportYomitan)
		r.Post("/import/jpdb-csv", importerHandler.ImportJPDBCSV)

		// FSRS optimizer
		r.Post("/settings/fsrs/optimize", settingsHandler.Optimize)

		// Output practice
		r.Get("/output/exercises", outputHandler.ListExercises)
		r.Post("/output/submit", outputHandler.Submit)
		r.Get("/output/shadowing", outputHandler.ShadowingQueue)
		r.Post("/output/shadowing/{id}/complete", outputHandler.CompleteShadowing)

	})

	// Stripe webhook — no auth middleware; signature verified inside handler.
	r.Post("/v1/billing/webhook", billingHandler.Webhook)

	// NLP proxy routes get their own subrouter without the global 30s timeout
	// middleware, since SudachiPy can take up to 2 minutes on first request.
	r.Route("/v1/nlp", func(r chi.Router) {
		r.Use(auth.Middleware)
		r.Post("/tokenize", nlpProxy.Tokenize)
		r.Post("/lookup", nlpProxy.Lookup)
		r.Post("/score-content", nlpProxy.ScoreContent)
		r.Post("/translate", nlpProxy.Translate)
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
		WriteTimeout: 130 * time.Second, // NLP proxy can take up to 120s on cold start
		IdleTimeout:  60 * time.Second,
	}

	// Daily word-count snapshot job — runs shortly after midnight UTC.
	go func() {
		for {
			now := time.Now().UTC()
			next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 5, 0, 0, time.UTC)
			time.Sleep(time.Until(next))
			stats.SnapshotAllUsers(pool)
		}
	}()

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
