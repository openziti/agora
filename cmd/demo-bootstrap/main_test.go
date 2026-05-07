package main

import (
	"testing"
	"time"
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
	if demoAccount.Password != "Agora-Demo-1" || demoAccount.Role != "admin" {
		t.Fatalf("unexpected demo account: %#v", demoAccount)
	}
}
