package server_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"orchis-go/internal/agentproto"
	"orchis-go/internal/server"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestEndpointRefreshHeartbeatConsentAndRetry(t *testing.T) {
	f := newAgentFixture(t)
	secret, asset := f.approvedAgent(t, "stream-host", "osquery.network_ranges")
	beat := func(s string) resp {
		return f.agentCall("POST", "heartbeat", map[string]any{"version": "1.0.0", "protocol_version": 1, "control_protocol": 2}, s)
	}
	if r := beat(secret); r.Status != 200 {
		t.Fatalf("heartbeat: %d %s", r.Status, r.Body)
	}
	refresh := f.a.do("POST", "/api/v1/assets/"+asset+"/endpoint-data/refresh", map[string]any{"query_key": "osquery.network_ranges"}, f.analyst)
	if refresh.Status != 202 {
		t.Fatalf("refresh: %d %s", refresh.Status, refresh.Body)
	}
	var ex execJSON
	refresh.JSON(&ex)
	if r := f.agentCall("POST", "executions/claim", nil, secret); r.Status != 204 {
		t.Fatalf("collect request bypassed heartbeat: %d", r.Status)
	}
	var hb struct {
		Command *agentproto.ControlCommand `json:"command"`
	}
	r := beat(secret)
	r.JSON(&hb)
	if hb.Command == nil || hb.Command.ID != ex.ID || hb.Command.Kind != "collect" || len(hb.Command.Queries) != 1 || hb.Command.Queries[0] != "osquery.network_ranges" {
		t.Fatalf("bad command: %+v", hb.Command)
	}
	r = beat(secret)
	r.JSON(&hb)
	if hb.Command == nil || hb.Command.ID != ex.ID {
		t.Fatal("unacknowledged command was not retried")
	}
	r = f.agentCall("POST", "heartbeat", map[string]any{"version": "1.0.0", "protocol_version": 1, "control_protocol": 3}, secret)
	hb.Command = nil
	r.JSON(&hb)
	if hb.Command == nil || hb.Command.ID != ex.ID {
		t.Fatal("collect-only agents must receive commands")
	}
	// A different agent cannot claim this request.
	other, _ := f.authorizedAgent(t, "other-host")
	if r = f.agentCall("POST", "executions/"+ex.ID+"/claim", nil, other); r.Status != 404 {
		t.Fatalf("cross-agent claim: %d", r.Status)
	}
	if r = f.agentCall("POST", "executions/"+ex.ID+"/claim", nil, secret); r.Status != 200 {
		t.Fatalf("claim: %d %s", r.Status, r.Body)
	}
	r = beat(secret)
	hb.Command = nil
	r.JSON(&hb)
	if hb.Command != nil {
		t.Fatal("claimed command was sent again")
	}
	data := map[string]any{"exit_code": 0, "datasets": []map[string]any{{"query": "osquery.network_ranges", "status": "ok", "rows": []map[string]any{{"interface": "eth0", "address": "192.0.2.3", "mask": "24", "source": "interface"}}}}}
	if r = f.agentCall("POST", "executions/"+ex.ID+"/result", data, secret); r.Status != 204 {
		t.Fatalf("result: %d %s", r.Status, r.Body)
	}
	if n := rowCount(t, f, `SELECT count(*) FROM endpoint_datasets WHERE asset_id=$1 AND query_key='osquery.network_ranges' AND execution_id=$2 AND last_status='ok'`, asset, ex.ID); n != 1 {
		t.Fatal("result not associated with request")
	}
}

func TestEndpointRefreshRejectsRevokedOrOffline(t *testing.T) {
	f := newAgentFixture(t)
	secret, asset := f.approvedAgent(t, "revoked-host", "osquery.network_ranges")
	ctx := context.Background()
	refresh := func() resp {
		return f.a.do("POST", "/api/v1/assets/"+asset+"/endpoint-data/refresh", map[string]any{"query_key": "osquery.network_ranges"}, f.analyst)
	}
	r := refresh()
	if r.Status != 202 {
		t.Fatalf("refresh: %d %s", r.Status, r.Body)
	}
	var ex execJSON
	r.JSON(&ex)
	if _, err := f.a.env.Owner.Exec(ctx, `UPDATE osquery_org_settings SET enabled=false WHERE organization_id=(SELECT organization_id FROM assets WHERE id=$1) AND query_key='osquery.network_ranges'`, asset); err != nil {
		t.Fatal(err)
	}
	beat := f.agentCall("POST", "heartbeat", map[string]any{"version": "1.0.0", "protocol_version": 1, "control_protocol": 2}, secret)
	var hb struct {
		Command *agentproto.ControlCommand `json:"command"`
	}
	beat.JSON(&hb)
	if hb.Command != nil {
		t.Fatal("revoked request dispatched")
	}
	claim := f.agentCall("POST", "executions/"+ex.ID+"/claim", nil, secret)
	if claim.Status == 200 {
		t.Fatal("revoked query claimed")
	}
	if n := rowCount(t, f, `SELECT count(*) FROM executions WHERE id=$1 AND state='running'`, ex.ID); n != 0 {
		t.Fatal("revoked query running")
	}
	if r = refresh(); r.Status != 409 {
		t.Fatalf("disabled refresh: %d %s", r.Status, r.Body)
	}
	if _, err := f.a.env.Owner.Exec(ctx, `UPDATE agents SET last_heartbeat_at=now()-interval '5 minutes' WHERE asset_id=$1`, asset); err != nil {
		t.Fatal(err)
	}
	if r = refresh(); r.Status != 409 || !strings.Contains(strings.ToLower(string(r.Body)), "offline") {
		t.Fatalf("offline refresh: %d %s", r.Status, r.Body)
	}
}

func TestEndpointStreamAuthEventsAndDisconnect(t *testing.T) {
	f := newAgentFixture(t)
	_, asset := f.approvedAgent(t, "stream-events", "osquery.network_ranges")
	path := "/api/v1/assets/" + asset + "/endpoint-data/stream"
	if r := f.a.do("GET", path, nil, ""); r.Status != 401 {
		t.Fatalf("anonymous: %d", r.Status)
	}
	if r := f.a.do("GET", path, nil, f.viewer); r.Status != 403 {
		t.Fatalf("viewer: %d", r.Status)
	}
	if r := f.a.do("GET", "/api/v1/assets/00000000-0000-0000-0000-000000000000/endpoint-data/stream", nil, f.analyst); r.Status != 404 {
		t.Fatalf("other asset: %d", r.Status)
	}
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", f.a.srv.URL+path, nil)
	req.AddCookie(&http.Cookie{Name: "orchis_session", Value: f.analyst})
	client := http.Client{Timeout: 10 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	defer cancel()
	reader := bufio.NewReader(response.Body)
	line, err := reader.ReadString('\n')
	if err != nil || !strings.Contains(line, "endpoint-data-ready") {
		t.Fatalf("ready: %q %v", line, err)
	}
	// Drain the ready event, then commit a real dataset update.
	for {
		l, e := reader.ReadString('\n')
		if e != nil {
			t.Fatal(e)
		}
		if l == "\n" {
			break
		}
	}
	other := uuid.New()
	if _, err := f.a.env.Owner.Exec(context.Background(), `INSERT INTO assets(id,tenant_id,name,category) VALUES($1,$2,'Other tenant','host')`, other, f.a.env.TenantB); err != nil {
		t.Fatal(err)
	}
	if _, err := f.a.env.Owner.Exec(context.Background(), `INSERT INTO endpoint_datasets(tenant_id,asset_id,query_key,last_status) VALUES($1,$2,'osquery.network_ranges','ok')`, f.a.env.TenantB, other); err != nil {
		t.Fatal(err)
	}
	_, err = f.a.env.Owner.Exec(context.Background(), `INSERT INTO endpoint_datasets(tenant_id,asset_id,query_key,last_status) VALUES($1,$2,'osquery.network_ranges','ok')`, f.a.env.TenantA, asset)
	if err != nil {
		t.Fatal(err)
	}
	line, err = reader.ReadString('\n')
	if err != nil || !strings.Contains(line, "endpoint-data-updated") {
		t.Fatalf("event: %q %v", line, err)
	}
	line, err = reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	var event map[string]any
	if err = json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data: "))), &event); err != nil {
		t.Fatal(err)
	}
	if event["asset_id"] != asset || event["query_key"] != "osquery.network_ranges" || event["tenant_id"] != nil {
		t.Fatalf("bad payload: %v", event)
	}
	cancel()
	_, _ = io.Copy(io.Discard, response.Body)
}

func TestMinuteSchedulerCollectsEveryEnabledWindowsQueryWithoutDuplicates(t *testing.T) {
	f := newAgentFixture(t)
	_, asset := f.approvedAgent(t, "windows-collector", "osquery.network_ranges", "osquery.network", "osquery.active_connections")
	ctx := context.Background()
	if _, err := f.a.env.Owner.Exec(ctx, `UPDATE agents SET os='windows',arch='x86_64' WHERE asset_id=$1`, asset); err != nil {
		t.Fatal(err)
	}
	// Stale, unknown settings that sort first must not fill the scheduler's
	// batch and starve valid Windows queries.
	if _, err := f.a.env.Owner.Exec(ctx, `INSERT INTO osquery_org_settings
 (tenant_id,organization_id,query_key,enabled,consent_version,consent_sha256,consented_by,consented_at)
 SELECT s.tenant_id,s.organization_id,'a.retired.'||n,true,s.consent_version,s.consent_sha256,s.consented_by,s.consented_at
 FROM osquery_org_settings s JOIN assets a ON a.tenant_id=s.tenant_id AND a.organization_id=s.organization_id
 CROSS JOIN generate_series(1,101) n
 WHERE a.id=$1 AND s.query_key='osquery.network_ranges'`, asset); err != nil {
		t.Fatal(err)
	}
	svc := server.EndpointService(server.Options{DB: f.a.env.App})
	if err := svc.CollectTick(ctx, f.a.env.TenantA, time.Minute); err != nil {
		t.Fatal(err)
	}
	count := func(query string) int { return rowCount(t, f, query, asset) }
	n := count(`SELECT count(*) FROM endpoint_collection_schedule s JOIN agents a ON a.tenant_id=s.tenant_id AND a.id=s.agent_id WHERE a.asset_id=$1 AND s.query_key IN ('osquery.network_ranges','osquery.network','osquery.active_connections')`)
	if n != 3 {
		t.Fatalf("enabled Windows queries scheduled: %d", n)
	}
	if n = count(`SELECT count(*) FROM endpoint_collection_schedule s JOIN agents a ON a.tenant_id=s.tenant_id AND a.id=s.agent_id WHERE a.asset_id=$1 AND s.query_key='osquery.users'`); n != 0 {
		t.Fatal("disabled query scheduled")
	}
	if _, err := f.a.env.Owner.Exec(ctx, `UPDATE endpoint_collection_schedule SET last_enqueued_at=now()-interval '61 seconds' WHERE agent_id IN(SELECT id FROM agents WHERE asset_id=$1)`, asset); err != nil {
		t.Fatal(err)
	}
	if err := svc.CollectTick(ctx, f.a.env.TenantA, time.Minute); err != nil {
		t.Fatal(err)
	}
	if n = count(`SELECT count(*) FROM executions WHERE target_asset_id=$1 AND state IN ('ready','running')`); n != 3 {
		t.Fatalf("overlapping executions: %d", n)
	}
}
