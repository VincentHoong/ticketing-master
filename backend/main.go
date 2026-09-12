package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"ticketing-master/config"
	"ticketing-master/handler"
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

	repositories, err := repository.NewRepositories(context.Background(), cfg)
	if err != nil {
		return err
	}
	defer repositories.Close()

	services := service.NewServices(repositories)

	r := handler.NewHandler(cfg, services, repositories)

	srv := newServer(cfg, r)
	serverErr := make(chan error, 1)
	go func() {
		err := srv.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
	}

	log.Println("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	return nil
}

func newServer(cfg *config.Config, r *chi.Mux) *http.Server {
	log.Printf("Server starting on :%s", cfg.Port)
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
		log.Fatal(err)
	}
}
