package server_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/people"
)

type personJSON struct {
	ID        string
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     *string
	Version   int
	Teams     []struct{ ID, Name string }
}

type teamJSON struct {
	ID, Name, Kind string
	MemberCount    int `json:"member_count"`
	Version        int
	Members        []struct{ ID, Name string }
}

func mkPerson(t *testing.T, a *app, cookie, first, email string) personJSON {
	t.Helper()
	body := map[string]any{"first_name": first, "last_name": "Test"}
	if email != "" {
		body["email"] = email
	}
	r := a.do("POST", "/api/v1/people", body, cookie)
	if r.Status != 201 {
		t.Fatalf("buat orang: %d %s", r.Status, r.Body)
	}
	var p personJSON
	r.JSON(&p)
	return p
}

func TestPeopleAndTeams(t *testing.T) {
	a, env := setup(t)
	analyst, viewer := a.login("analyst@a.test"), a.login("viewer@a.test")
	ada := mkPerson(t, a, analyst, "Ada", "ada@corp.test")
	bob := mkPerson(t, a, analyst, "Bob", "")
	if r := a.do("POST", "/api/v1/people", map[string]any{"first_name": "X", "email": "ADA@corp.test"}, analyst); r.Status != 422 {
		t.Fatalf("email ganda (beda huruf) harus 422: %d %s", r.Status, r.Body)
	}
	if r := a.do("POST", "/api/v1/people", map[string]any{"first_name": " ", "email": "no-at"}, analyst); r.Status != 422 {
		t.Fatalf("masukan salah: %d", r.Status)
	}
	if r := a.do("POST", "/api/v1/people", map[string]any{"first_name": "Z"}, viewer); r.Status != 403 {
		t.Fatalf("viewer tidak boleh menulis: %d", r.Status)
	}
	// pencarian; wildcard di-escape
	var l struct{ Items []personJSON }
	a.do("GET", "/api/v1/people?q=corp.test", nil, analyst).JSON(&l)
	if len(l.Items) != 1 || l.Items[0].FirstName != "Ada" {
		t.Fatalf("cari email: %+v", l)
	}
	a.do("GET", "/api/v1/people?q=%25", nil, analyst).JSON(&l)
	if len(l.Items) != 0 {
		t.Fatalf("%% harus huruf biasa: %+v", l)
	}
	// ubah dengan kunci optimistik
	if r := a.do("PATCH", "/api/v1/people/"+ada.ID, map[string]any{"version": ada.Version, "first_name": "Ada", "last_name": "Lovelace", "organization": "Corp"}, analyst); r.Status != 200 {
		t.Fatalf("patch: %d %s", r.Status, r.Body)
	}
	if r := a.do("PATCH", "/api/v1/people/"+ada.ID, map[string]any{"version": ada.Version, "first_name": "Ada"}, analyst); r.Status != 409 {
		t.Fatalf("versi usang: %d", r.Status)
	}
	// tim
	r := a.do("POST", "/api/v1/teams", map[string]any{"name": "Blue team", "kind": "internal"}, analyst)
	if r.Status != 201 {
		t.Fatalf("buat tim: %d %s", r.Status, r.Body)
	}
	var tm teamJSON
	r.JSON(&tm)
	if r := a.do("POST", "/api/v1/teams", map[string]any{"name": "Blue team"}, analyst); r.Status != 422 {
		t.Fatalf("nama tim ganda: %d", r.Status)
	}
	if r := a.do("POST", "/api/v1/teams", map[string]any{"name": "x", "kind": "alien"}, analyst); r.Status != 422 {
		t.Fatalf("jenis tim: %d", r.Status)
	}
	r = a.do("PUT", "/api/v1/teams/"+tm.ID+"/members", map[string]any{"version": tm.Version, "person_ids": []string{ada.ID, bob.ID, ada.ID}}, analyst)
	if r.Status != 200 {
		t.Fatalf("anggota: %d %s", r.Status, r.Body)
	}
	r.JSON(&tm)
	if tm.MemberCount != 2 || len(tm.Members) != 2 {
		t.Fatalf("duplikat id dihitung sekali: %+v", tm)
	}
	if r := a.do("PUT", "/api/v1/teams/"+tm.ID+"/members", map[string]any{"version": tm.Version - 1, "person_ids": []string{}}, analyst); r.Status != 409 {
		t.Fatalf("versi tim usang: %d", r.Status)
	}
	if r := a.do("PUT", "/api/v1/teams/"+tm.ID+"/members", map[string]any{"version": tm.Version, "person_ids": []string{uuid.NewString()}}, analyst); r.Status != 422 {
		t.Fatalf("orang tak ada: %d", r.Status)
	}
	var one personJSON
	a.do("GET", "/api/v1/people/"+ada.ID, nil, analyst).JSON(&one)
	if len(one.Teams) != 1 || one.Teams[0].Name != "Blue team" {
		t.Fatalf("tim milik orang: %+v", one)
	}
	// menghapus orang melepasnya dari tim
	if r := a.do("DELETE", "/api/v1/people/"+bob.ID, nil, analyst); r.Status != 204 {
		t.Fatalf("hapus orang: %d", r.Status)
	}
	var leftover int
	env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM team_members WHERE person_id=$1`, bob.ID).Scan(&leftover)
	if leftover != 0 {
		t.Fatalf("keanggotaan tim orang yang dihapus harus dibersihkan: %d", leftover)
	}
	var tl struct{ Items []teamJSON }
	a.do("GET", "/api/v1/teams", nil, analyst).JSON(&tl)
	if len(tl.Items) != 1 || tl.Items[0].MemberCount != 1 {
		t.Fatalf("jumlah anggota setelah hapus: %+v", tl)
	}
	if r := a.do("DELETE", "/api/v1/teams/"+tm.ID, nil, analyst); r.Status != 204 {
		t.Fatalf("hapus tim: %d", r.Status)
	}
	if r := a.do("GET", "/api/v1/teams/"+tm.ID, nil, analyst); r.Status != 404 {
		t.Fatalf("tim terhapus: %d", r.Status)
	}
	// isolasi tenant
	svc := &people.Service{DB: env.App}
	pb := &auth.Principal{TenantID: env.TenantB, Permissions: []string{authz.PeopleRead}}
	got, err := svc.ListPeople(context.Background(), pb, "")
	if err != nil || len(got) != 0 {
		t.Fatalf("tenant B tidak boleh melihat orang tenant A: %+v %v", got, err)
	}
	if _, err := svc.GetPerson(context.Background(), pb, uuid.MustParse(ada.ID)); err == nil {
		t.Fatal("tenant B tidak boleh membaca orang tenant A")
	}
}
