package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/r1pu5u/pentest-harnes/internal/goribridge"
)

func main() {
	var c goribridge.Config
	flag.StringVar(&c.DB, "db", "", "Engagement Gori SQLite database")
	flag.StringVar(&c.Events, "events", "", "Append-only Gori capture JSONL")
	flag.StringVar(&c.State, "state", "", "Private delivery checkpoint JSON")
	flag.StringVar(&c.ScopeFile, "scope-file", "", "Harness state.json containing authorized scope")
	flag.StringVar(&c.IngestURL, "ingest-url", "http://127.0.0.1:3333/caido-ingest", "Direct JXScout ingestion endpoint")
	flag.StringVar(&c.Gori, "gori", "gori", "Gori executable")
	flag.DurationVar(&c.Poll, "poll", time.Second, "Capture polling interval")
	flag.IntVar(&c.HistoryLimit, "history-limit", 100000, "Maximum history rows for startup recovery; larger projects fail explicitly")
	flag.BoolVar(&c.Once, "once", false, "Process current history/events once and exit")
	flag.Parse()
	b, err := goribridge.New(c)
	if err == nil {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		err = b.Run(ctx)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, goribridge.CleanError(err))
		os.Exit(1)
	}
}
