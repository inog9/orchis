package contract

import (
	"encoding/json"
	"testing"
)

func TestSchemaRejectsTypesUnknownFieldsAndTrailingJSON(t *testing.T) {
	for _, raw := range []string{`null`, `[]`, `{} {}`, `{"profile":null}`, `{"service_detection":"true"}`, `{"ports":22}`, `{"profile":"web","shell":"id"}`} {
		if _, e := Normalize(KeyNmapTCP, json.RawMessage(raw)); e == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}
func TestCatalogDefinitionCanReuseAdapterWithoutNewUI(t *testing.T) {
	d := *Find(KeyNmapTCP)
	d.Key = "nmap.inventory"
	if e := d.ValidateDefinition(); e != nil {
		t.Fatal(e)
	}
	Defs = append(Defs, d)
	defer func() { Defs = Defs[:len(Defs)-1] }()
	raw, e := Normalize(d.Key, json.RawMessage(`{"profile":"web"}`))
	if e != nil || len(raw) == 0 {
		t.Fatalf("new definition %s %v", raw, e)
	}
	d.Adapter = "shell"
	if d.ValidateDefinition() == nil {
		t.Fatal("arbitrary executor accepted")
	}
}
func TestVersionedDefinitionsKeepFrozenParameterDefaults(t *testing.T) {
	data, _ := json.Marshal(Find(KeyNmapTCP))
	var next Def
	json.Unmarshal(data, &next)
	next.Version = 2
	p := next.Schema.Properties["profile"]
	p.Default = "web"
	next.Schema.Properties["profile"] = p
	Defs = append(Defs, next)
	defer func() { Defs = Defs[:len(Defs)-1] }()
	old, e := NormalizeVersion(KeyNmapTCP, 1, json.RawMessage(`{}`))
	if e != nil {
		t.Fatal(e)
	}
	newer, e := NormalizeVersion(KeyNmapTCP, 2, json.RawMessage(`{}`))
	if e != nil {
		t.Fatal(e)
	}
	if string(old) == string(newer) {
		t.Fatal("versioned defaults were not isolated")
	}
	if _, e = NormalizeVersion(KeyNmapTCP, 99, json.RawMessage(`{}`)); e == nil {
		t.Fatal("unsupported definition version accepted")
	}
}
