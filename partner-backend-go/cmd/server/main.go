package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jiguangliandong/superapp-embed-demo/partner-backend-go/internal/config"
	partnerserver "github.com/jiguangliandong/superapp-embed-demo/partner-backend-go/internal/server"
)

func main() {
	logger := log.New(os.Stdout, "partner-backend-go ", log.LstdFlags|log.LUTC)
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal(err)
	}
	application, err := partnerserver.New(cfg, logger)
	if err != nil {
		logger.Fatal(err)
	}

	httpServer := &http.Server{
		Addr:              cfg.ListenAddress,
		Handler:           application.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	stopContext, stop := signal.NotifyContext(
		context.Background(), os.Interrupt, syscall.SIGTERM,
	)
	defer stop()
	go func() {
		<-stopContext.Done()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownContext); err != nil {
			logger.Printf("HTTP shutdown failed: %v", err)
		}
	}()

	logger.Printf("listening on http://localhost%s", cfg.ListenAddress)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatal(err)
	}
}
