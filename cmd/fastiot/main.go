package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"

	"fastiot/internal/app"
)

var version = "development"

func main() {
	port := flag.String("port", "8080", "HTTP port")
	dataDir := flag.String("data", "data", "data directory")
	natsPort := flag.Int("nats-port", 4222, "embedded NATS port")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	application, err := app.Start(ctx, app.Config{
		DataDir:  *dataDir,
		HTTPPort: *port,
		NATSPort: *natsPort,
		Version:  version,
	})
	if err != nil {
		log.Fatal(err)
	}

	select {
	case <-ctx.Done():
	case <-application.Exited():
	}
	if err := application.Close(); err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}
