package catalog_test

import (
	"context"
	"testing"

	"github.com/openziti/agora/sdk/agent"
	"github.com/openziti/agora/sdk/agent/catalog"
)

func ExampleEnsurePublished() {
	var a *agent.Agent
	_, _ = catalog.EnsurePublished(context.Background(), a, catalog.PublishSpec{
		Name: "llm-gateway",
		Capabilities: []catalog.Capability{
			{Name: "llm-routing"},
		},
		WorkgroupScopeIDs: []string{"wg_abcdefghijkl"},
		TunnelMode:        catalog.TunnelTCP,
		ContractID:        "con_abcdefghijkl",
	})
}

func TestPublicSurfaceCompilesWithoutInternalAPI(t *testing.T) {
	var publish func(context.Context, *agent.Agent, catalog.PublishSpec) (*catalog.Advertisement, error)
	publish = catalog.EnsurePublished
	if publish == nil {
		t.Fatal("expected publish function")
	}
}
