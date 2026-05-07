package main

import (
	"testing"
	"time"

	"github.com/openziti/agora/internal/api"
)

func TestParseDemoDurationSupportsDaySuffix(t *testing.T) {
	got, err := parseDemoDuration("7d")
	if err != nil {
		t.Fatalf("parse duration: %v", err)
	}
	if got != 7*24*time.Hour {
		t.Fatalf("expected 7d, got %s", got)
	}
}

func TestEmbeddedTopologyShape(t *testing.T) {
	topo, err := loadTopology("")
	if err != nil {
		t.Fatalf("load topology: %v", err)
	}
	if len(topo.Organizations) != 6 {
		t.Fatalf("expected 6 organizations, got %d", len(topo.Organizations))
	}
	if len(topo.Accounts) != 11 {
		t.Fatalf("expected 11 accounts, got %d", len(topo.Accounts))
	}
	if len(topo.Workgroups) != 8 {
		t.Fatalf("expected 8 workgroups, got %d", len(topo.Workgroups))
	}

	var newsContract string
	var demoAccount accountSpec
	for _, acct := range topo.Accounts {
		switch acct.Email {
		case "news-pulse@signals-co":
			newsContract = acct.Advertisement.Contract
		case "demo@agora.local":
			demoAccount = acct
		}
	}
	if newsContract != "demo-contract-tight" {
		t.Fatalf("news-pulse contract = %q, want demo-contract-tight", newsContract)
	}
	if demoAccount.Password != "Agora-Demo-1" || demoAccount.Role != "admin" || !demoAccount.Env {
		t.Fatalf("unexpected demo account: %#v", demoAccount)
	}
}

func TestDemoAccountCreateRequest(t *testing.T) {
	topo, err := loadTopology("")
	if err != nil {
		t.Fatalf("load topology: %v", err)
	}
	demoAccount, ok := findAccount(topo, "demo@agora.local")
	if !ok {
		t.Fatalf("demo account not found")
	}

	req, err := createAccountRequest(demoAccount)
	if err != nil {
		t.Fatalf("create account request: %v", err)
	}
	if req.Email != "demo@agora.local" {
		t.Fatalf("email = %q", req.Email)
	}
	if req.Password != "Agora-Demo-1" {
		t.Fatalf("password = %q", req.Password)
	}
	role, ok := req.Role.Get()
	if !ok || role != api.CreateAccountRequestRoleAdmin {
		t.Fatalf("role = %q set=%v", role, ok)
	}
	displayName, ok := req.DisplayName.Get()
	if !ok || displayName != "Agora Demo" {
		t.Fatalf("display name = %q set=%v", displayName, ok)
	}
}

func TestDemoAccountIsInterOrgChannelAdmin(t *testing.T) {
	topo, err := loadTopology("")
	if err != nil {
		t.Fatalf("load topology: %v", err)
	}
	want := map[string]bool{
		"markets-channel":   false,
		"weather-channel":   false,
		"signals-channel":   false,
		"analytics-channel": false,
	}
	for _, wg := range topo.Workgroups {
		if _, ok := want[wg.Name]; !ok {
			continue
		}
		for _, invitation := range wg.InvitedOrganizations {
			if invitation.Organization == "enterprise-client" && invitation.InitialAdmin == "demo@agora.local" {
				want[wg.Name] = true
			}
		}
	}
	for name, ok := range want {
		if !ok {
			t.Fatalf("demo account is not invited admin for %s", name)
		}
	}
}

func findAccount(topo *topology, email string) (accountSpec, bool) {
	for _, acct := range topo.Accounts {
		if acct.Email == email {
			return acct, true
		}
	}
	return accountSpec{}, false
}
