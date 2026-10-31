package main

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/db"
	"orchis-go/internal/mq"
	"orchis-go/internal/telemetry"
	"os"
	"path/filepath"
	"time"
)

func collectMetrics(ctx context.Context, m *telemetry.Registry, d *db.DB, tenant uuid.UUID, amqpURL string) {
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	for {
		sampleCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		err := d.InTenantRead(sampleCtx, tenant, func(tx pgx.Tx) error {
			values := map[string]string{
				"orchis_database_bytes":                     "SELECT pg_database_size(current_database())::float8",
				"orchis_scan_failed_records":                "SELECT count(*)::float8 FROM executions WHERE outcome IN ('error','timeout')",
				"orchis_jobs_stuck{kind=\"execution\"}":     "SELECT count(*)::float8 FROM executions WHERE (state='running' AND lease_until<now()) OR (state='ready' AND not_before<now()-interval '5 minutes')",
				"orchis_jobs_stuck{kind=\"report\"}":        "SELECT count(*)::float8 FROM reports WHERE (status='rendering' AND lease_until<now()) OR (status='queued' AND created_at<now()-interval '5 minutes')",
				"orchis_jobs_stuck{kind=\"ai\"}":            "SELECT count(*)::float8 FROM ai_jobs WHERE (status='running' AND lease_until<now()) OR (status='queued' AND created_at<now()-interval '5 minutes')",
				"orchis_agent_heartbeat_oldest_age_seconds": "SELECT COALESCE(max(extract(epoch FROM now()-last_heartbeat_at)),0)::float8 FROM agents WHERE status!='revoked'",
				"orchis_agents_without_heartbeat":           "SELECT count(*)::float8 FROM agents WHERE status!='revoked' AND last_heartbeat_at IS NULL",
			}
			sample := map[string]float64{}
			for name, query := range values {
				var value float64
				if err := tx.QueryRow(sampleCtx, query).Scan(&value); err != nil {
					return err
				}
				sample[name] = value
			}
			for _, kind := range []string{"nmap", "nuclei"} {
				var age float64
				var missing float64
				if err := tx.QueryRow(sampleCtx, "SELECT COALESCE(max(extract(epoch FROM now()-last_heartbeat_at)),0)::float8,count(*) FILTER(WHERE last_heartbeat_at IS NULL)::float8 FROM integrations WHERE key=$1 AND enabled", kind).Scan(&age, &missing); err != nil {
					return err
				}
				sample["orchis_injector_heartbeat_age_seconds{kind=\""+kind+"\"}"] = age
				sample["orchis_injector_missing_heartbeat{kind=\""+kind+"\"}"] = missing
			}
			for name, value := range sample {
				m.Set(name, value)
			}
			return nil
		})
		cancel()
		if err != nil {
			m.Set("orchis_metrics_database_up", 0)
		} else {
			m.Set("orchis_metrics_database_up", 1)
			m.Set("orchis_metrics_database_last_success_timestamp_seconds", float64(time.Now().Unix()))
		}
		if amqpURL != "" {
			sampleQueues(m, amqpURL)
		}
		if path := os.Getenv("ORCHIS_OPS_STATUS_FILE"); path != "" {
			m.Disk(filepath.Dir(path))
			var state struct {
				Success   bool    `json:"success"`
				Timestamp float64 `json:"timestamp"`
				Free      float64 `json:"disk_free_bytes"`
				Total     float64 `json:"disk_total_bytes"`
			}
			data, err := os.ReadFile(path)
			if err == nil && len(data) < 4096 && json.Unmarshal(data, &state) == nil {
				m.Set("orchis_backup_status_known", 1)
				m.Set("orchis_backup_last_attempt_timestamp_seconds", state.Timestamp)
				m.Set("orchis_backup_last_success", 0)
				if state.Success {
					m.Set("orchis_backup_last_success", 1)
				}
				// disk is sampled from the status directory mount, not the archived value.
				_ = state.Free
				_ = state.Total
			} else {
				m.Set("orchis_backup_status_known", 0)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
func sampleQueues(m *telemetry.Registry, url string) {
	m.Set("orchis_metrics_queue_up", 0)
	c, err := mq.Dial(url)
	if err != nil {
		return
	}
	defer c.Close()
	timer := time.AfterFunc(5*time.Second, func() { c.Close() })
	defer timer.Stop()
	ch, err := c.Channel()
	if err != nil {
		return
	}
	defer ch.Close()
	for _, name := range append(append([]string{}, mq.Queues...), mq.DeadQ) {
		q, err := ch.QueueInspect(name)
		if err != nil {
			return
		}
		m.Set("orchis_queue_ready_messages{queue=\""+name+"\"}", float64(q.Messages))
		m.Set("orchis_queue_consumers{queue=\""+name+"\"}", float64(q.Consumers))
	}
	m.Set("orchis_metrics_queue_up", 1)
}
