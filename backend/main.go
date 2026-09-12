package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"ticketing-master/config"
	"ticketing-master/handler"
	"ticketing-master/logging"
	"ticketing-master/repository"
	"ticketing-master/service"
	"time"

	"github.com/go-chi/chi/v5"
)

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := logging.New(cfg.LogLevel)
	slog.SetDefault(logger)

	ctx := logging.WithContext(context.Background(), logger)
	repositories, err := repository.NewRepositories(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer repositories.Close()

	services := service.NewServices(repositories)

	r := handler.NewHandler(cfg, services, repositories, logger)

	srv := newServer(cfg, r, logger)
	serverErr := make(chan error, 1)
	go func() {
		err := srv.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	stopCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-serverErr:
		return err
	case <-stopCtx.Done():
	}

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	return nil
}

func newServer(cfg *config.Config, r *chi.Mux, logger *slog.Logger) *http.Server {
	logger.Info("server starting", "port", cfg.Port)
	return &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}
