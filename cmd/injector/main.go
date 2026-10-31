// Pekerja injector: injector --kind nmap|nuclei
// Lingkungan: ORCHIS_SERVER_URL, ORCHIS_INJECTOR_TOKEN, ORCHIS_AMQP_URL, ORCHIS_ALLOW_LOOPBACK_TARGETS, NUCLEI_TEMPLATES_DIR.
package main

import (
	"context"
	"flag"
	"log/slog"
	"orchis-go/internal/envfile"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"orchis-go/internal/contract"
	"orchis-go/internal/injector"
	"orchis-go/internal/telemetry"
)

var version = "dev"

func main() {
	if err := envfile.Load(); err != nil {
		fatal(err.Error())
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	kind := flag.String("kind", "", "nmap | nuclei")
	flag.Parse()
	if *kind != "nmap" && *kind != "nuclei" {
		fatal("gunakan --kind nmap atau --kind nuclei")
	}
	server, token, amqpURL := os.Getenv("ORCHIS_SERVER_URL"), os.Getenv("ORCHIS_INJECTOR_TOKEN"), os.Getenv("ORCHIS_AMQP_URL")
	if server == "" || token == "" || amqpURL == "" {
		fatal("ORCHIS_SERVER_URL, ORCHIS_INJECTOR_TOKEN, dan ORCHIS_AMQP_URL wajib diisi")
	}
	loop, _ := strconv.ParseBool(os.Getenv("ORCHIS_ALLOW_LOOPBACK_TARGETS"))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	metrics, err := telemetry.Start(ctx)
	if err != nil {
		fatal(err.Error())
	}
	(&injector.Worker{
		Metrics: metrics, Kind: *kind, Client: injector.NewClient(server, token), AMQPURL: amqpURL, Version: version,
		Guard: contract.Guard{AllowLoopback: loop}, TemplatesDir: os.Getenv("NUCLEI_TEMPLATES_DIR"),
	}).Run(ctx)
}

func fatal(msg string) {
	slog.Error(msg)
	os.Exit(1)
}
