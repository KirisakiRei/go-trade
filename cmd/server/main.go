package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"ila-engine/internal/config"
	"ila-engine/internal/model"
	"ila-engine/internal/router"
)

func main() {
	_ = godotenv.Load("/opt/data/ila-engine/.env")
	// 1. Structured JSON Logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("Starting ILA Trading Engine (Imaginary Liquidity)...")

	// 2. Load Config
	cfg := config.LoadConfig()

	// 3. Init SQLite Database
	db, err := config.InitDatabase(cfg.DBPath)
	if err != nil {
		slog.Error("Fatal database initialization error", "error", err)
		os.Exit(1)
	}

	// Auto-migrate tables
	if err := db.AutoMigrate(&model.TradeJournal{}, &model.MarketCandle{}, &model.Position{}); err != nil {
		slog.Error("Fatal database migration error", "error", err)
		os.Exit(1)
	}
	slog.Info("Database tables migrated successfully", "db_path", cfg.DBPath)

	// 4. Setup Router & Wiring
	app := router.SetupRouter(cfg, db)

	// Background worker: Real-time position monitor (Passive Broker Reconciliation)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("Recovered from panic in background position monitor", "panic", r)
			}
		}()
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = app.ExecutionService.CheckAndManagePositions(ctx)
			cancel()
		}
	}()

	// 5. Start HTTP Server with Graceful Shutdown
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      app.Engine,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("HTTP Server listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("Server listen failed", "error", err)
		}
	}()

	// 6. Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down server gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
	}

	slog.Info("Server stopped cleanly")
}
