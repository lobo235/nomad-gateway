package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/lobo235/nomad-gateway/internal/nomad"
)

// Server holds the dependencies for the HTTP server.
type Server struct {
	nomad   *nomad.Client
	apiKey  string
	log     *slog.Logger
	version string
}

func NewServer(nomadClient *nomad.Client, apiKey, version string, log *slog.Logger) *Server {
	return &Server{
		nomad:   nomadClient,
		apiKey:  apiKey,
		log:     log,
		version: version,
	}
}

// Handler builds and returns the root http.Handler with all routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	auth := bearerAuth(s.apiKey)

	// /health is unauthenticated — used by Nomad container health checks
	mux.HandleFunc("GET /health", s.healthHandler())

	// Authenticated routes
	mux.Handle("GET /jobs", auth(http.HandlerFunc(s.listJobsHandler())))
	mux.Handle("GET /jobs/{jobID}", auth(http.HandlerFunc(s.getJobHandler())))
	mux.Handle("GET /jobs/{jobID}/spec", auth(http.HandlerFunc(s.getJobSpecHandler())))
	mux.Handle("GET /jobs/{jobID}/evaluations", auth(http.HandlerFunc(s.getEvaluationsHandler())))
	mux.Handle("GET /jobs/{jobID}/versions", auth(http.HandlerFunc(s.getJobVersionsHandler())))
	mux.Handle("POST /jobs/{jobID}/revert", auth(http.HandlerFunc(s.revertJobHandler())))
	mux.Handle("GET /jobs/{jobID}/allocations", auth(http.HandlerFunc(s.getAllocationsHandler())))
	mux.Handle("GET /jobs/{jobID}/allocations/{allocID}", auth(http.HandlerFunc(s.getAllocInfoHandler())))
	mux.Handle("POST /jobs/{jobID}/allocations/{allocID}/restart", auth(http.HandlerFunc(s.restartAllocHandler())))
	mux.Handle("GET /jobs/{jobID}/allocations/{allocID}/logs", auth(http.HandlerFunc(s.getLogsHandler())))
	mux.Handle("GET /node-pools", auth(http.HandlerFunc(s.listNodePoolsHandler())))
	mux.Handle("GET /node-pools/{poolName}/nodes", auth(http.HandlerFunc(s.listNodesInPoolHandler())))
	mux.Handle("POST /jobs", auth(http.HandlerFunc(s.submitJobHandler())))
	mux.Handle("DELETE /jobs/{jobID}", auth(http.HandlerFunc(s.stopJobHandler())))
	mux.Handle("GET /jobs/{jobID}/health", auth(http.HandlerFunc(s.watchJobHealthHandler())))

	return requestLogger(s.log)(mux)
}

// Run starts the HTTP server and blocks until ctx is cancelled, then shuts down gracefully.
func (s *Server) Run(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:         addr,
		Handler:      s.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 10*time.Minute + 15*time.Second, // must exceed max health-watch timeout
		IdleTimeout:  60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		s.log.Info("server listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("server error: %w", err)
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		s.log.Info("shutting down server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
