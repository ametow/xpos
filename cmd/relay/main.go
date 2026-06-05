package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ametow/xpos/relay/xpos"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	relay := xpos.New()
	if err := relay.Init(); err != nil {
		logger.Error("init failed", "err", err.Error())
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM,
	)
	defer stop()

	if err := relay.Start(ctx); err != nil {
		logger.Error("relay exited with error", "err", err.Error())
		os.Exit(1)
	}
	logger.Info("relay stopped cleanly")
}
