package server_test

import "testing"

type alertList struct {
	Items []struct {
		ID, Kind, Severity, Title string
		Read                      bool
	}
	Unread int
}

func getAlerts(t *testing.T, a *app, cookie, query string) alertList {
	t.Helper()
	var l alertList
	r := a.do("GET", "/api/v1/alerts"+query, nil, cookie)
	if r.Status != 200 {
		t.Fatalf("alerts: %d %s", r.Status, r.Body)
	}
	r.JSON(&l)
	return l
}

func TestAlertsForCriticalFindingsFailedScansAndSimulations(t *testing.T) {
	c := newChainFixture(t)
	c.mustReady(t, []map[string]any{step("scan", nmapC, "after_parents", nil), step("web", nucleiC, "on_finding", onPort("tcp/80"))}, [][2]string{{"scan", "web"}})
	sim := c.launch(t)
	c.finish(t, c.execsOf(t, sim.ID, "scan")[0], nmapC, ports("tcp/80"))
	c.finish(t, c.execsOf(t, sim.ID, "web")[0], nucleiC, vuln("critical"))

	l := getAlerts(t, c.a, c.analyst, "")
	kinds := map[string]int{}
	for _, a := range l.Items {
		kinds[a.Kind]++
	}
	if kinds["new_critical_finding"] != 1 || kinds["simulation_finished"] != 1 || l.Unread != 2 {
		t.Fatalf("alert setelah simulasi: %v unread=%d", kinds, l.Unread)
	}
	viewerAlerts := getAlerts(t, c.a, c.a.login("viewer@a.test"), "")
	if len(viewerAlerts.Items) != 1 || viewerAlerts.Items[0].Kind != "new_critical_finding" {
		t.Fatal("Viewer finding notification audience missing", viewerAlerts)
	}
	// hasil kritis yang sama dilaporkan lagi pada run berikut: tidak ada alert ganda (dedupe)
	c.a.do("POST", "/api/v1/atomic-tests", map[string]any{"name": "again", "contract_key": nucleiC, "target_asset_id": c.asset}, c.admin)
	var tl struct{ Items []struct{ ID string } }
	c.a.do("GET", "/api/v1/atomic-tests", nil, c.admin).JSON(&tl)
	var ex execJSON
	c.a.do("POST", "/api/v1/atomic-tests/"+tl.Items[0].ID+"/run", nil, c.admin).JSON(&ex)
	c.finish(t, ex.ID, nucleiC, vuln("critical"))
	if n := len(getAlerts(t, c.a, c.analyst, "").Items); n != 2 {
		t.Fatalf("temuan kritis yang sama tidak boleh membuat alert ganda: %d", n)
	}
	// scan gagal -> alert peringatan
	var ex2 execJSON
	c.a.do("POST", "/api/v1/atomic-tests/"+tl.Items[0].ID+"/run", nil, c.admin).JSON(&ex2)
	c.finish(t, ex2.ID, nucleiC, failed)
	l = getAlerts(t, c.a, c.analyst, "")
	if len(l.Items) != 3 || l.Items[0].Kind != "test_failed" || l.Items[0].Severity != "warning" {
		t.Fatalf("alert scan gagal: %+v", l.Items)
	}

	// baca, baca-semua, singkirkan: status per pengguna
	if r := c.a.do("POST", "/api/v1/alerts/"+l.Items[0].ID+"/read", nil, c.analyst); r.Status != 204 {
		t.Fatalf("read: %d", r.Status)
	}
	mine := getAlerts(t, c.a, c.analyst, "")
	if mine.Unread != 2 || !mine.Items[0].Read {
		t.Fatalf("setelah dibaca: %+v", mine)
	}
	if other := getAlerts(t, c.a, c.admin, ""); other.Unread != 3 {
		t.Fatalf("status baca milik pengguna lain tidak berubah: %+v", other)
	}
	if only := getAlerts(t, c.a, c.analyst, "?unread=1"); len(only.Items) != 2 {
		t.Fatalf("filter belum dibaca: %+v", only)
	}
	if r := c.a.do("POST", "/api/v1/alerts/read-all", nil, c.analyst); r.Status != 204 {
		t.Fatalf("read-all: %d", r.Status)
	}
	if got := getAlerts(t, c.a, c.analyst, ""); got.Unread != 0 || len(got.Items) != 3 {
		t.Fatalf("setelah baca semua: %+v", got)
	}
	if r := c.a.do("POST", "/api/v1/alerts/"+l.Items[0].ID+"/dismiss", nil, c.analyst); r.Status != 204 {
		t.Fatalf("dismiss: %d", r.Status)
	}
	if got := getAlerts(t, c.a, c.analyst, ""); len(got.Items) != 2 {
		t.Fatalf("alert yang disingkirkan hilang dari daftar pengguna itu: %+v", got)
	}
	if got := getAlerts(t, c.a, c.admin, ""); len(got.Items) != 3 {
		t.Fatalf("tetap terlihat bagi pengguna lain: %+v", got)
	}
	if r := c.a.do("POST", "/api/v1/alerts/00000000-0000-0000-0000-000000000000/read", nil, c.analyst); r.Status != 404 {
		t.Fatalf("alert tak dikenal: %d", r.Status)
	}
	if r := c.a.do("GET", "/api/v1/alerts", nil, ""); r.Status != 401 {
		t.Fatalf("tanpa login: %d", r.Status)
	}
}
