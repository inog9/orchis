package server_test

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/agents"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/endpoint"
	"orchis-go/internal/httpx"
	"orchis-go/internal/osquery"
	"orchis-go/internal/server"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAgentControlsManualRolloutCASAndAuthenticatedUninstall(t *testing.T) {
	f := newAgentFixture(t)
	one, _ := f.enroll(t, "control-one")
	two, _ := f.enroll(t, "control-two")
	publish := func(version, content string) {
		dir := makeRelease(t, version, content)
		for _, file := range []string{"agent-linux-amd64", "manifest.json"} {
			b, e := os.ReadFile(filepath.Join(dir, file))
			if e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(filepath.Join(f.release, file), b, 0644); e != nil {
				t.Fatal(e)
			}
		}
	}
	base := "/api/v1/agents/" + one.AgentID
	control := func(action string, version int, confirmation string) resp {
		return f.a.do("POST", base+"/control", map[string]any{"action": action, "version": version, "confirmation": confirmation}, f.admin)
	}
	for _, path := range []string{base + "/control", base + "/collect"} {
		if r := f.a.do("POST", path, map[string]any{"action": "pause", "version": 1}, f.analyst); r.Status != 403 {
			t.Fatal("Analyst control allowed", r.Status)
		}
	}
	if r := f.a.do("POST", base+"/control", map[string]any{"action": "pause", "version": 1}, f.admin, "X-Requested-With", ""); r.Status != 403 {
		t.Fatal("CSRF control allowed", r.Status)
	}
	if r := control("uninstall", 1, one.AgentID); r.Status != 409 {
		t.Fatal("old agent must not pretend uninstall support", r.Status, string(r.Body))
	}
	heartbeat := func(secret, version string) resp {
		return f.agentCall("POST", "heartbeat", map[string]any{"version": version, "protocol_version": 1, "control_protocol": 1, "osquery_version": "5.20.0", "diagnostic_code": "unexpected secret path /root/nope", "last_update_error": "oen_DO_NOT_PERSIST"}, secret)
	}
	if r := heartbeat(one.Secret, "1.0.0"); r.Status != 200 {
		t.Fatal(r.Status, string(r.Body))
	}
	if r := heartbeat(two.Secret, "1.0.0"); r.Status != 200 {
		t.Fatal(r.Status, string(r.Body))
	}
	// A heartbeat must never race an administrator by incrementing control_version.
	if r := control("pause", 1, ""); r.Status != 200 {
		t.Fatal("heartbeat changed control CAS", r.Status, string(r.Body))
	}
	if r := control("resume", 1, ""); r.Status != 409 {
		t.Fatal("stale control accepted", r.Status)
	}
	r := heartbeat(one.Secret, "1.0.0")
	var beat agents.HeartbeatReply
	r.JSON(&beat)
	if !beat.Paused {
		t.Fatal("pause missing from heartbeat")
	}
	if r = control("resume", 2, ""); r.Status != 200 {
		t.Fatal(r.Status, string(r.Body))
	}
	r = f.a.do("PUT", "/api/v1/agents/rollout", agents.RolloutPolicy{Mode: "manual", Version: 0}, f.admin)
	if r.Status != 200 {
		t.Fatal(r.Status, string(r.Body))
	}
	if r = f.a.do("PUT", "/api/v1/agents/rollout", agents.RolloutPolicy{Mode: "automatic", Version: 0}, f.admin); r.Status != 409 {
		t.Fatal("stale rollout accepted", r.Status)
	}
	publish("1.1.0", "NEXT-FAKE-BINARY")
	r = heartbeat(one.Secret, "1.0.0")
	r.JSON(&beat)
	if beat.LatestVersion != "" {
		t.Fatal("manual rollout leaked release")
	}
	if r = control("update", 3, ""); r.Status != 200 {
		t.Fatal(r.Status, string(r.Body))
	}
	beat = agents.HeartbeatReply{}
	r = heartbeat(one.Secret, "1.0.0")
	r.JSON(&beat)
	if beat.LatestVersion != "1.1.0" {
		t.Fatal("requested release not offered", string(r.Body))
	}
	beat = agents.HeartbeatReply{}
	r = heartbeat(two.Secret, "1.0.0")
	r.JSON(&beat)
	if beat.LatestVersion != "" {
		t.Fatal("other agent received manual release")
	}
	if r = heartbeat(one.Secret, "1.1.0"); r.Status != 200 {
		t.Fatal(r.Status)
	}
	if r = control("update", 4, ""); r.Status != 409 {
		t.Fatal("equal release update allowed", r.Status)
	}
	publish("1.0.0", "OLDER-FAKE-BINARY")
	if r = control("update", 4, ""); r.Status != 409 {
		t.Fatal("downgrade allowed", r.Status)
	}
	if r = control("uninstall", 4, "wrong"); r.Status != 422 {
		t.Fatal("uninstall name guard missing", r.Status)
	}
	if r = control("uninstall", 4, one.AgentID); r.Status != 200 {
		t.Fatal(r.Status, string(r.Body))
	}
	var command string
	for i := 0; i < 2; i++ {
		beat = agents.HeartbeatReply{}
		r = heartbeat(one.Secret, "1.1.0")
		r.JSON(&beat)
		if beat.Command == nil || beat.Command.Kind != "uninstall" || beat.Command.AgentID != one.AgentID || !beat.Paused {
			t.Fatal("missing bound command", string(r.Body))
		}
		if i == 0 {
			command = beat.Command.ID
		} else if command != beat.Command.ID {
			t.Fatal("command changed on retry")
		}
	}
	if r = control("resume", 5, ""); r.Status != 409 {
		t.Fatal("pending uninstall resumed", r.Status)
	}
	ack := map[string]any{"id": command, "success": true}
	if r = f.agentCall("POST", "control/ack", ack, two.Secret); r.Status != 409 {
		t.Fatal("cross-agent ACK accepted", r.Status)
	}
	if r = f.a.do("POST", "/api/v1/agent/control/ack", ack, f.admin); r.Status != 401 {
		t.Fatal("cookie ACK accepted", r.Status)
	}
	if r = f.agentCall("POST", "control/ack", map[string]any{"id": uuid.NewString(), "success": true}, one.Secret); r.Status != 409 {
		t.Fatal("wrong ACK accepted", r.Status)
	}
	if r = f.agentCall("POST", "control/ack", ack, one.Secret); r.Status != 204 {
		t.Fatal(r.Status, string(r.Body))
	}
	if r = heartbeat(one.Secret, "1.1.0"); r.Status != 401 {
		t.Fatal("uninstalled credential still valid", r.Status)
	}
	if r = heartbeat(two.Secret, "1.0.0"); r.Status != 200 {
		t.Fatal("other agent revoked", r.Status)
	}
	r = f.a.do("GET", "/api/v1/agents", nil, f.admin)
	if strings.Contains(string(r.Body), "DO_NOT_PERSIST") || strings.Contains(string(r.Body), "/root/nope") {
		t.Fatal("unsafe diagnostic persisted")
	}
	var list struct{ Items []agents.Agent }
	r.JSON(&list)
	for _, a := range list.Items {
		if a.ID.String() == one.AgentID && (a.UninstallStatus != "completed" || a.Status != "revoked" || a.UninstallCompletedAt == nil) {
			t.Fatal("uninstall completion not persisted", a)
		}
	}
	var details string
	if e := f.a.env.Owner.QueryRow(context.Background(), `SELECT COALESCE(string_agg(detail::text,''),'') FROM audit_log WHERE action LIKE 'agent.%'`).Scan(&details); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(details, "DO_NOT_PERSIST") || strings.Contains(details, one.Secret) {
		t.Fatal("secret in audit")
	}
	var state string
	if e := f.a.env.Owner.QueryRow(context.Background(), `SELECT status FROM agents WHERE id=$1`, two.AgentID).Scan(&state); e != nil || state == "revoked" {
		t.Fatal("other agent changed", state, e)
	}
}

func TestAgentPauseBlocksQueuedClaimNewRunAndEveryCollectionPath(t *testing.T) {
	f := newAgentFixture(t)
	secret, asset := f.authorizedAgent(t, "pause-collection")
	org := f.joinOrg(t, asset)
	ctx := context.Background()
	var agentID string
	if e := f.a.env.Owner.QueryRow(ctx, `SELECT id FROM agents WHERE asset_id=$1`, asset).Scan(&agentID); e != nil {
		t.Fatal(e)
	}
	controlPath := "/api/v1/agents/" + agentID + "/control"
	collectPath := "/api/v1/agents/" + agentID + "/collect"
	if r := f.a.do("POST", collectPath, nil, f.admin); r.Status != 409 {
		t.Fatal("collection without consent allowed", r.Status)
	}
	in := endpoint.CollectionInput{Platform: "linux", Keys: []string{"osquery.inventory"}, Versions: map[string]int64{}, Consent: osquery.Consent().ConsentRef}
	for _, q := range osquery.Queries {
		if q.SupportsOS("linux") {
			in.Versions[q.Key] = 0
		}
	}
	if r := f.a.do("PUT", "/api/v1/organizations/"+org+"/endpoint-collection", in, f.admin); r.Status != 204 {
		t.Fatal(r.Status, string(r.Body))
	}
	queued := f.createRun(t, secret, asset, "osquery.inventory")
	if r := f.a.do("POST", controlPath, agents.ControlInput{Action: "pause", Version: 1}, f.admin); r.Status != 200 {
		t.Fatal(r.Status, string(r.Body))
	}
	if r := f.agentCall("POST", "executions/claim", nil, secret); r.Status != 204 {
		t.Fatal("paused agent claimed queued work", r.Status, string(r.Body))
	}
	// Direct Runtime.Claim must enforce the same check, not just the HTTP handler.
	rt := server.Runtime(server.Options{DB: f.a.env.App, Tenant: f.a.env.TenantA})
	svc := server.AgentService(server.Options{DB: f.a.env.App, Tenant: f.a.env.TenantA})
	principal, e := svc.Authenticate(ctx, secret)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = rt.Claim(ctx, principal.Worker(), uuid.MustParse(queued)); e == nil {
		t.Fatal("direct claim bypassed pause")
	}
	if r := f.a.do("POST", collectPath, nil, f.admin); r.Status != 409 {
		t.Fatal("paused manual collect allowed", r.Status)
	}
	// Use a fresh definition so the already-busy guard cannot mask pause.
	var fresh struct{ ID string }
	created := f.a.do("POST", "/api/v1/atomic-tests", map[string]any{"name": "Pause queue probe", "contract_key": "osquery.inventory", "target_asset_id": asset}, f.analyst)
	if created.Status != 201 {
		t.Fatal(created.Status, string(created.Body))
	}
	created.JSON(&fresh)
	if r := f.a.do("POST", "/api/v1/atomic-tests/"+fresh.ID+"/run", nil, f.analyst); r.Status != 409 || !strings.Contains(string(r.Body), "paused") {
		t.Fatal("paused new run allowed", r.Status, string(r.Body))
	}
	endpointSvc := server.EndpointService(server.Options{DB: f.a.env.App})
	// Remove queued work from the due-time decision: pause itself must block collection.
	if _, e = f.a.env.Owner.Exec(ctx, `UPDATE executions SET state='ended',outcome='canceled',ended_at=now() WHERE id=$1`, queued); e != nil {
		t.Fatal(e)
	}
	if e = endpointSvc.CollectTick(ctx, f.a.env.TenantA, 15*time.Minute); e != nil {
		t.Fatal("paused periodic collection must skip cleanly", e)
	}
	var count int
	if e = f.a.env.Owner.QueryRow(ctx, `SELECT count(*) FROM endpoint_collection_schedule WHERE agent_id=$1`, agentID).Scan(&count); e != nil || count != 0 {
		t.Fatal("paused auto collection queued", count, e)
	}
	if r := f.a.do("POST", controlPath, agents.ControlInput{Action: "resume", Version: 2}, f.admin); r.Status != 200 {
		t.Fatal(r.Status, string(r.Body))
	}
	collect := f.a.do("POST", collectPath, nil, f.admin)
	if collect.Status != 202 {
		t.Fatal(collect.Status, string(collect.Body))
	}
	var first, second struct {
		IDs []string `json:"execution_ids"`
	}
	collect.JSON(&first)
	again := f.a.do("POST", collectPath, nil, f.admin)
	again.JSON(&second)
	if again.Status != 202 || len(first.IDs) != 1 || len(second.IDs) != 1 || first.IDs[0] != second.IDs[0] {
		t.Fatal("collection duplicated pending query", string(collect.Body), string(again.Body))
	}
	if r := f.agentCall("POST", "executions/claim", nil, secret); r.Status != 200 {
		t.Fatal("resumed collection not claimable", r.Status, string(r.Body))
	}
	var current struct{ Version int }
	f.a.do("GET", "/api/v1/assets/"+asset, nil, f.admin).JSON(&current)
	if r := f.a.do("PATCH", "/api/v1/assets/"+asset, map[string]any{"version": current.Version, "testing_authorized": false}, f.admin); r.Status != 200 {
		t.Fatal(r.Status, string(r.Body))
	}
	if r := f.a.do("POST", collectPath, nil, f.admin); r.Status != 409 {
		t.Fatal("collection after authorization revoked allowed", r.Status)
	}
	f.a.do("GET", "/api/v1/assets/"+asset, nil, f.admin).JSON(&current)
	if r := f.a.do("PATCH", "/api/v1/assets/"+asset, map[string]any{"version": current.Version, "testing_authorized": true}, f.admin); r.Status != 200 {
		t.Fatal(r.Status, string(r.Body))
	}
	if r := f.a.do("PUT", "/api/v1/organizations/"+org+"/osquery/osquery.inventory", map[string]any{"version": 1, "enabled": false, "consent": osquery.Consent().ConsentRef}, f.admin); r.Status != 204 {
		t.Fatal(r.Status, string(r.Body))
	}
	if r := f.a.do("POST", collectPath, nil, f.admin); r.Status != 409 {
		t.Fatal("collection after consent revoked allowed", r.Status)
	}

}

func TestAgentControlTenantIsolationAndRelatedPermissions(t *testing.T) {
	f := newAgentFixture(t)
	one, _ := f.enroll(t, "isolated-control")
	ctx := context.Background()
	svc := server.AgentService(server.Options{DB: f.a.env.App, Tenant: f.a.env.TenantA})
	var actor uuid.UUID
	if e := f.a.env.Owner.QueryRow(ctx, `SELECT id FROM users WHERE tenant_id=$1 AND email='admin@a.test'`, f.a.env.TenantA).Scan(&actor); e != nil {
		t.Fatal(e)
	}
	admin := &auth.Principal{TenantID: f.a.env.TenantA, UserID: actor, Permissions: authz.SystemRoles[0].Permissions}
	for _, missing := range []string{authz.SettingsManage, authz.AgentManage} {
		narrow := *admin
		narrow.Permissions = nil
		for _, v := range admin.Permissions {
			if v != missing {
				narrow.Permissions = append(narrow.Permissions, v)
			}
		}
		_, e := svc.Control(ctx, &narrow, uuid.MustParse(one.AgentID), agents.ControlInput{Action: "pause", Version: 1}, "", "narrow")
		var problem *httpx.Problem
		if !errors.As(e, &problem) || problem.Status != 403 {
			t.Fatal("missing related permission allowed", missing, e)
		}
	}
	foreign := *admin
	foreign.TenantID = f.a.env.TenantB
	_, e := svc.Control(ctx, &foreign, uuid.MustParse(one.AgentID), agents.ControlInput{Action: "pause", Version: 1}, "", "foreign")
	var problem *httpx.Problem
	if !errors.As(e, &problem) || problem.Status != 404 {
		t.Fatal("cross-tenant control accepted", e)
	}
	if _, e = svc.SetRollout(ctx, admin, agents.RolloutPolicy{Mode: "manual"}, "", "fixture"); e != nil {
		t.Fatal(e)
	}
	other, e := svc.Rollout(ctx, &foreign)
	if e != nil || other.Mode != "automatic" || other.Version != 0 {
		t.Fatal("cross-tenant rollout policy leaked", other, e)
	}
	var n int
	if e = f.a.env.App.InTenantRead(ctx, f.a.env.TenantB, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM agent_rollout_policies WHERE tenant_id=$1`, f.a.env.TenantA).Scan(&n)
	}); e != nil || n != 0 {
		t.Fatal("rollout tenant RLS missing", n, e)
	}
}
