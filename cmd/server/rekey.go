package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"orchis-go/internal/config"
	"orchis-go/internal/db"
	"orchis-go/internal/server"
	"os"
	"strings"
	"time"
)

func rekeyAI(ctx context.Context, cfg *config.Config) error {
	path := os.Getenv("ORCHIS_AI_REKEY_NEW_FILE")
	if path == "" {
		return errors.New("new AI master key file is required")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 128 {
		return errors.New("new AI master key must be a private regular file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return errors.New("cannot read new AI master key")
	}
	next, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(next) != 32 {
		return errors.New("invalid new AI master key")
	}
	defer clear(next)
	run, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	return withTenant(run, cfg, func(d *db.DB, tenant uuid.UUID) error {
		n, err := server.AIService(server.Options{DB: d, Tenant: tenant}).Rekey(run, next)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]int{"reencrypted_policies": n})
	})
}
