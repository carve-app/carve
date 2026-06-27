package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/carve-app/carve/services/api/internal/auth"
	"github.com/carve-app/carve/services/api/internal/billing"
	"github.com/carve-app/carve/services/api/internal/cards"
	"github.com/carve-app/carve/services/api/internal/db"
	"github.com/carve-app/carve/services/api/internal/decks"
	"github.com/carve-app/carve/services/api/internal/discover"
	"github.com/carve-app/carve/services/api/internal/export"
	"github.com/carve-app/carve/services/api/internal/grammar"
	"github.com/carve-app/carve/services/api/internal/immersion"
	"github.com/carve-app/carve/services/api/internal/importer"
	"github.com/carve-app/carve/services/api/internal/library"
	"github.com/carve-app/carve/services/api/internal/metrics"
	"github.com/carve-app/carve/services/api/internal/nlp"
	"github.com/carve-app/carve/services/api/internal/onboarding"
	"github.com/carve-app/carve/services/api/internal/output"
	"github.com/carve-app/carve/services/api/internal/reports"
	"github.com/carve-app/carve/services/api/internal/review"
	"github.com/carve-app/carve/services/api/internal/settings"
	"github.com/carve-app/carve/services/api/internal/stats"
	syncbridge "github.com/carve-app/carve/services/api/internal/sync"
	"github.com/carve-app/carve/services/api/internal/users"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(log)

	// Fail fast on a missing/weak signing secret rather than silently falling
	// back to the published dev default (which would allow token forgery).
	if err := auth.RequireJWTSecret(); err != nil {
		slog.Error("refusing to start", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := db.Connect(ctx)
	if err != nil {
		slog.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	slog.Info("database connected")
	r := newRouter(pool)
	discoverIngester := discover.NewIngester(pool)
	runServer(pool, r, discoverIngester)
}

// newRouter owns HTTP composition independently from process startup and
// background jobs, allowing route/middleware behavior to be tested in-process.
func newRouter(pool *pgxpool.Pool) chi.Router {
	r := chi.NewRouter()
	r.Use(chimw.RealIP)
	r.Use(chimw.RequestID)
	r.Use(metrics.HTTPMiddleware)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins(),
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
	r.Get("/health/live", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"status":"ok","service":"api"}`)
	})
	r.Get("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(ctx); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintln(w, `{"status":"unavailable","service":"api","dependency":"database"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"status":"ready","service":"api"}`)
	})

	// Prometheus metrics are open in local development and bearer-protected
	// whenever METRICS_TOKEN is configured (required by production deploys).
	r.Get("/metrics", metrics.ProtectedHandler)

	// Auth routes (no auth middleware)
	authHandler := auth.NewHandler(pool)
	r.With(chimw.Timeout(30*time.Second)).Route("/v1/auth", func(r chi.Router) {
		r.Post("/register", authHandler.Register)
		r.Post("/login", authHandler.Login)
		r.Post("/refresh", authHandler.Refresh)
		r.Post("/logout", authHandler.Logout)
		r.Post("/forgot", authHandler.ForgotPassword)
		r.Post("/reset", authHandler.ResetPassword)
		r.Post("/verify", authHandler.VerifyEmail)
		r.Post("/verify/resend", authHandler.ResendVerification)
	})

	// Protected routes
	userHandler := users.NewHandler(pool)
	cardsHandler := cards.NewHandler(pool)
	reviewHandler := review.NewHandler(pool)
	immersionHandler := immersion.NewHandler(pool)
	nlpProxy := nlp.NewProxy(pool)
	nlpExplain := nlp.NewExplainHandler(pool)
	decksHandler := decks.NewHandler(pool)
	exportHandler := export.NewHandler(pool)
	settingsHandler := settings.NewHandler(pool)
	statsHandler := stats.NewHandler(pool)
	libraryHandler := library.NewHandler(pool)
	importerHandler := importer.NewHandler(pool)
	outputHandler := output.NewHandler(pool)
	onboardingHandler := onboarding.NewHandler(pool)
	syncHandler := syncbridge.NewHandler(pool)
	discoverHandler := discover.NewHandler(pool)
	reportsHandler := reports.NewHandler(pool)
	grammarHandler := grammar.NewHandler(pool)

	r.With(chimw.Timeout(30*time.Second)).Route("/v1", func(r chi.Router) {
		r.Use(auth.Middleware)

		// Users
		r.Get("/users/me", userHandler.Me)
		r.Patch("/users/me", userHandler.Update)
		r.Delete("/users/me", userHandler.Delete)

		// Onboarding
		r.Get("/onboarding/placement-test", onboardingHandler.PlacementTest)
		r.Post("/onboarding/placement-test", onboardingHandler.SubmitPlacementTest)
		r.Post("/onboarding/known-words", onboardingHandler.KnownWords)
		r.Post("/onboarding/starter-deck", onboardingHandler.StarterDeck)

		// Cards
		r.Post("/cards", cardsHandler.Create)
		r.Post("/cards/bulk", cardsHandler.Bulk)
		r.Post("/cards/find-similar", cardsHandler.FindSimilar)
		r.Get("/cards", cardsHandler.List)
		r.Get("/cards/{id}", cardsHandler.Get)
		r.Patch("/cards/{id}", cardsHandler.Update)
		r.Delete("/cards/{id}", cardsHandler.Delete)
		r.Post("/cards/{id}/media", cardsHandler.AttachMedia)
		r.Post("/cards/{id}/suspend", cardsHandler.Suspend)
		r.Post("/cards/{id}/unsuspend", cardsHandler.Unsuspend)
		r.Post("/cards/{id}/bury", cardsHandler.Bury)
		r.Post("/cards/{id}/unbury", cardsHandler.Unbury)

		// Review
		r.Get("/review/due-count", reviewHandler.DueCount)
		r.Get("/review/session", reviewHandler.Session)
		r.Post("/review/events", reviewHandler.SubmitEvent)
		r.Post("/review/undo", reviewHandler.Undo)
		r.Get("/review/intervals", reviewHandler.Intervals)
		r.Get("/review/forecast", reviewHandler.Forecast)
		r.Get("/review/notifications", reviewHandler.Notifications)
		r.Post("/review/notifications/{id}/read", reviewHandler.MarkNotificationRead)

		// Decks
		r.Get("/decks", decksHandler.List)
		r.Post("/decks", decksHandler.Create)
		r.Post("/decks/generate", decksHandler.Generate)
		r.Patch("/decks/{id}", decksHandler.Update)
		r.Delete("/decks/{id}", decksHandler.DeleteDeck)
		r.Post("/decks/{id}/subscribe", decksHandler.Subscribe)
		r.Delete("/decks/{id}/subscribe", decksHandler.Unsubscribe)
		r.Post("/decks/{id}/rate", decksHandler.Rate)

		// Export
		r.Get("/export", exportHandler.Export)
		r.Get("/export/csv", exportHandler.ExportCSV)
		r.Get("/export/apkg", exportHandler.ExportAPKG)

		// Settings
		r.Get("/settings/fsrs", settingsHandler.GetFSRS)
		r.Put("/settings/fsrs", settingsHandler.PutFSRS)
		r.Get("/settings/workload-preview", settingsHandler.WorkloadPreview)

		// Immersion
		r.Post("/immersion", immersionHandler.Create)

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
		r.Post("/output/transcribe", outputHandler.Transcribe)

		// AnkiConnect bridge
		r.Post("/sync/anki-connect/test", syncHandler.Test)
		r.Post("/sync/anki-connect", syncHandler.Sync)

		// Discover (content recommendations)
		r.Get("/discover/feed", discoverHandler.Feed)

		// Learning reports
		r.Get("/reports/weekly", reportsHandler.Weekly)

		// Grammar — user's known JLPT patterns (catalog lives behind /v1/nlp/grammar/patterns)
		r.Get("/grammar/known", grammarHandler.ListKnown)
		r.Post("/grammar/known", grammarHandler.MarkKnown)
		r.Delete("/grammar/known", grammarHandler.UnmarkKnown)

	})

	// Billing is disabled during the free-alpha milestone: the web UI has no
	// checkout/portal and no endpoint enforces subscription tiers. Registering
	// the routes only when Stripe is configured keeps the unfinished payment
	// surface (incl. the webhook) off the public API by default. Set
	// STRIPE_SECRET_KEY to re-enable post-alpha.
	if os.Getenv("STRIPE_SECRET_KEY") != "" {
		billingHandler := billing.NewHandler(pool)
		r.Group(func(r chi.Router) {
			r.Use(chimw.Timeout(30 * time.Second))
			r.Use(auth.Middleware)
			r.Get("/v1/billing/subscription", billingHandler.Subscription)
			r.Post("/v1/billing/checkout", billingHandler.Checkout)
		})
		// Stripe webhook — no auth middleware; signature verified inside handler.
		r.Post("/v1/billing/webhook", billingHandler.Webhook)
	}

	// NLP proxy routes get a longer deadline because SudachiPy can take up to
	// two minutes on a cold dictionary/tokenizer start.
	r.With(chimw.Timeout(125*time.Second)).Route("/v1/nlp", func(r chi.Router) {
		r.Use(auth.Middleware)
		r.Post("/tokenize", nlpProxy.Tokenize)
		r.Post("/lookup", nlpProxy.Lookup)
		r.Post("/score-content", nlpProxy.ScoreContent)
		r.Post("/translate", nlpProxy.Translate)
		r.Post("/select-sentence", nlpProxy.SelectSentence)
		r.Get("/grammar/patterns", nlpProxy.GrammarPatterns)
		r.Post("/explain", nlpExplain.Explain)
		r.Get("/word-audio", nlpExplain.WordAudio)
		r.Get("/word-image", nlpProxy.WordImage)
	})
	return r
}

// runServer owns lifecycle concerns only: serving, cancellable background jobs,
// signal handling, and graceful shutdown.
func runServer(pool *pgxpool.Pool, handler http.Handler, discoverIngester *discover.Ingester) {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port

	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 130 * time.Second, // NLP proxy can take up to 120s on cold start
		IdleTimeout:  60 * time.Second,
	}

	stopJobs := startBackgroundJobs(context.Background(), pool, discoverIngester)
	defer stopJobs()

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
	stopJobs()

	slog.Info("shutting down gracefully")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	srv.Shutdown(shutCtx)
}

func allowedOrigins() []string {
	raw := os.Getenv("ALLOWED_ORIGINS")
	if raw == "" {
		return []string{
			"http://localhost:*", "https://localhost:*",
			"http://127.0.0.1:*", "https://127.0.0.1:*",
		}
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
