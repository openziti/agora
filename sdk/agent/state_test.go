package agent

import (
	"testing"

	"github.com/openziti/agora/environment/env_core"
)

func TestMatchesConnectIdentity(t *testing.T) {
	const listen = "127.0.0.1:8401"
	tests := []struct {
		name     string
		existing env_core.ManagedConnect
		tunnelID string
		refName  string
		address  string
		want     bool
	}{
		{"same ID", env_core.ManagedConnect{TunnelID: "tt_old", Name: "old", ListenAddress: listen}, "tt_old", "new", listen, true},
		{"different IDs with same name", env_core.ManagedConnect{TunnelID: "tt_new", Name: "shared", ListenAddress: listen}, "tt_old", "shared", listen, false},
		{"different address", env_core.ManagedConnect{TunnelID: "tt_old", Name: "shared", ListenAddress: listen}, "tt_old", "shared", "127.0.0.1:8402", false},
		{"unresolved saved name", env_core.ManagedConnect{Name: "Shared", ListenAddress: listen}, "tt_old", "shared", listen, true},
		{"name-only request", env_core.ManagedConnect{TunnelID: "tt_old", Name: "Shared", ListenAddress: listen}, "", "shared", listen, true},
		{"ID-only request does not match unresolved name", env_core.ManagedConnect{Name: "tt_old", ListenAddress: listen}, "tt_old", "", listen, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesConnect(tt.existing, tt.tunnelID, tt.refName, tt.address); got != tt.want {
				t.Fatalf("matchesConnect = %v, want %v", got, tt.want)
			}
		})
	}
}
