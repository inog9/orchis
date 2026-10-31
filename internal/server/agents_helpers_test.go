package server_test

import (
	"testing"

	"github.com/google/uuid"

	"orchis-go/internal/execution"
)

func mustUUID(t *testing.T, s string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustAgentWorker(t *testing.T, integration, agent string) *execution.Injector {
	t.Helper()
	a := mustUUID(t, agent)
	return &execution.Injector{ID: mustUUID(t, integration), Key: "agent", AgentID: &a}
}
