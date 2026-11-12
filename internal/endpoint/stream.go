package endpoint

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/auth"
	"orchis-go/internal/httpx"
)

// Dedicated connections are bounded and never consume the transactional pool.
var streamSlots = make(chan struct{}, 16)

type endpointEvent struct {
	TenantID    uuid.UUID  `json:"tenant_id"`
	AssetID     uuid.UUID  `json:"asset_id"`
	QueryKey    string     `json:"query_key"`
	DatasetID   string     `json:"dataset_id"`
	ExecutionID *uuid.UUID `json:"execution_id"`
	RequestID   *uuid.UUID `json:"request_id"`
	CollectedAt *time.Time `json:"collected_at"`
	Status      string     `json:"status"`
}

func (h *Handlers) stream(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	asset, err := id(r, "id", "Asset")
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	exists := func(ctx context.Context) (bool, error) {
		var ok bool
		err := h.Svc.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM assets WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL)`, p.TenantID, asset).Scan(&ok)
		})
		return ok, err
	}
	ok, err := exists(r.Context())
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	if !ok {
		httpx.Write(w, r, httpx.NotFound("Asset"))
		return
	}
	select {
	case streamSlots <- struct{}{}:
		defer func() { <-streamSlots }()
	default:
		httpx.Write(w, r, httpx.NewProblem(503, "Stream unavailable", "Too many endpoint subscriptions."))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	conn, err := pgx.ConnectConfig(ctx, h.Svc.DB.Pool.Config().ConnConfig.Copy())
	if err != nil {
		httpx.Write(w, r, httpx.NewProblem(503, "Stream unavailable", "Endpoint notifications are unavailable."))
		return
	}
	defer conn.Close(context.Background())
	if _, err = conn.Exec(ctx, "LISTEN orchis_endpoint_data"); err != nil {
		httpx.Write(w, r, httpx.NewProblem(503, "Stream unavailable", "Endpoint notifications are unavailable."))
		return
	}
	controller := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	send := func(event, data string) error {
		_ = controller.SetWriteDeadline(time.Now().Add(20 * time.Second))
		if _, e := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); e != nil {
			return e
		}
		return controller.Flush()
	}
	if err = send("endpoint-data-ready", "{}"); err != nil {
		return
	}
	for ctx.Err() == nil {
		wait, stop := context.WithTimeout(ctx, 15*time.Second)
		notification, e := conn.WaitForNotification(wait)
		stop()
		if e != nil {
			if ctx.Err() != nil {
				return
			}
			if e == context.DeadlineExceeded {
				if send("heartbeat", "{}") != nil {
					return
				}
				continue
			}
			return
		}
		var v endpointEvent
		if json.Unmarshal([]byte(notification.Payload), &v) != nil || v.TenantID != p.TenantID || v.AssetID != asset {
			continue
		}
		ok, err = exists(ctx)
		if err != nil || !ok {
			return
		}
		payload, e := json.Marshal(struct {
			AssetID     uuid.UUID  `json:"asset_id"`
			QueryKey    string     `json:"query_key"`
			DatasetID   string     `json:"dataset_id"`
			ExecutionID *uuid.UUID `json:"execution_id"`
			RequestID   *uuid.UUID `json:"request_id"`
			CollectedAt *time.Time `json:"collected_at"`
			Status      string     `json:"status"`
		}{v.AssetID, v.QueryKey, v.DatasetID, v.ExecutionID, v.RequestID, v.CollectedAt, v.Status})
		if e != nil || send("endpoint-data-updated", string(payload)) != nil {
			return
		}
	}
}
