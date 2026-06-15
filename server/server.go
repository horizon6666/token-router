package server

import (
	"context"
	"errors"
	"log"
	"net/http"

	"token-router/conf"
)

// Run starts the HTTP server and blocks until ctx is cancelled, then performs
// a graceful shutdown bounded by conf.GlobalConfigs.ShutdownTimeout.
func Run(ctx context.Context, handler http.Handler) error {
	cfg := conf.GlobalConfigs
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("token-router listening on %s (n=%d, m=%d)", cfg.Addr, cfg.Nodes, cfg.Budget)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		log.Println("shutting down...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return <-errCh
	case err := <-errCh:
		return err
	}
}
