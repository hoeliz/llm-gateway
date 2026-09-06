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

	"llm-gateway/internal/gateway"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	timeout := 2 * time.Minute
	if raw := os.Getenv("UPSTREAM_TIMEOUT"); raw != "" {
		var err error
		timeout, err = time.ParseDuration(raw)
		if err != nil || timeout <= 0 {
			slog.Error("UPSTREAM_TIMEOUT must be a positive duration")
			os.Exit(1)
		}
	}
	g, err := gateway.New(gateway.Config{APIKey: os.Getenv("GATEWAY_API_KEY"), OpenAIKey: os.Getenv("OPENAI_API_KEY"), AnthropicKey: os.Getenv("ANTHROPIC_API_KEY"), OpenAIURL: os.Getenv("OPENAI_BASE_URL"), AnthropicURL: os.Getenv("ANTHROPIC_BASE_URL"), Timeout: timeout})
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	server := &http.Server{Addr: addr, Handler: g.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: timeout + 5*time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	go func() { slog.Info("gateway listening", "address", addr); done <- server.ListenAndServe() }()
	select {
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
			slog.Error("shutdown deadline exceeded")
		}
	}
}
