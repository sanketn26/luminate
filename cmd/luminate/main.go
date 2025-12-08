package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/yourusername/luminate/pkg/api"
	"github.com/yourusername/luminate/pkg/auth"
	"github.com/yourusername/luminate/pkg/config"
	"github.com/yourusername/luminate/pkg/middleware"
	"github.com/yourusername/luminate/pkg/storage"
	"github.com/yourusername/luminate/pkg/storage/badger"
	"github.com/yourusername/luminate/pkg/storage/clickhouse"
)

var (
	configFile = flag.String("config", "configs/config.yaml", "Path to configuration file")
	version    = "dev"
	commit     = "unknown"
	buildTime  = "unknown"
)

func main() {
	flag.Parse()

	// Print version info
	fmt.Printf("Luminate v%s (commit: %s, built: %s)\n", version, commit, buildTime)

	// Load configuration
	cfg, err := config.Load(*configFile)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize storage backend
	var store storage.MetricsStore
	switch cfg.Storage.Backend {
	case "badger":
		store, err = badger.NewStore(cfg.Storage.Badger)
		if err != nil {
			log.Fatalf("Failed to initialize BadgerDB: %v", err)
		}
	case "clickhouse":
		store, err = clickhouse.NewStore(cfg.Storage.ClickHouse)
		if err != nil {
			log.Fatalf("Failed to initialize ClickHouse: %v", err)
		}
	default:
		log.Fatalf("Unknown storage backend: %s", cfg.Storage.Backend)
	}
	defer store.Close()

	// Initialize JWT authenticator
	authenticator, err := auth.NewJWTAuthenticator(cfg.Auth)
	if err != nil {
		log.Fatalf("Failed to initialize authenticator: %v", err)
	}

	// Create API handler
	handler := api.NewHandler(store, cfg)

	// Setup router with middleware
	router := http.NewServeMux()

	// Public endpoints
	router.HandleFunc("/api/v1/health", handler.Health)
	router.HandleFunc("/metrics", handler.Metrics)

	// Protected endpoints with authentication
	protectedMux := http.NewServeMux()
	protectedMux.HandleFunc("/api/v1/query", handler.Query)
	protectedMux.HandleFunc("/api/v1/write", handler.Write)
	protectedMux.HandleFunc("/api/v1/metrics", handler.ListMetrics)
	protectedMux.HandleFunc("/api/v1/metrics/", handler.MetricsDimensions)
	protectedMux.HandleFunc("/api/v1/admin/", handler.Admin)

	// Apply middleware chain
	var protectedHandler http.Handler = protectedMux
	if cfg.Auth.Enabled {
		protectedHandler = middleware.AuthMiddleware(authenticator)(protectedHandler)
	}
	protectedHandler = middleware.RateLimitMiddleware(cfg.RateLimits)(protectedHandler)
	protectedHandler = middleware.LoggingMiddleware(protectedHandler)
	protectedHandler = middleware.RecoveryMiddleware(protectedHandler)

	router.Handle("/api/v1/", protectedHandler)

	// Create HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Starting Luminate server on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}
