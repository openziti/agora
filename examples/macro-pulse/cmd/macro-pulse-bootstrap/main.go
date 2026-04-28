package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/openziti/agora/internal/api"
)

// Macro Pulse bootstrap: provisions the demo topology of 5 orgs,
// 9 agent accounts, and 7 workgroups (4 inter-org with auto-accepted
// invitations + 3 intra-org). Idempotent on 409 conflicts so it can
// be re-run against a partially-bootstrapped controller.
//
// Required env: AGORA_ADMIN_TOKEN. Default controller URL is
// http://127.0.0.1:8080; override with --controller.

const (
	orgMarkets    = "markets-co"
	orgWeather    = "weather-co"
	orgSignals    = "signals-co"
	orgAnalytics  = "analytics-co"
	orgEnterprise = "enterprise-client"
)

var agentsByOrg = map[string][]string{
	orgMarkets:    {"equity-feed@markets-co", "fx-feed@markets-co", "commodities-feed@markets-co"},
	orgWeather:    {"weather-feed@weather-co"},
	orgSignals:    {"search-trends@signals-co", "news-pulse@signals-co"},
	orgAnalytics:  {"correlator@analytics-co", "narrator@analytics-co"},
	orgEnterprise: {"pulse-agent@enterprise-client"},
}

type interOrgChannel struct {
	Name           string
	OwnerOrg       string
	OwnerAdminAcct string
	InvitedOrg     string
	InvitedAdmin   string
}

var interOrgChannels = []interOrgChannel{
	{Name: "markets-channel", OwnerOrg: orgMarkets, OwnerAdminAcct: "equity-feed@markets-co", InvitedOrg: orgEnterprise, InvitedAdmin: "pulse-agent@enterprise-client"},
	{Name: "weather-channel", OwnerOrg: orgWeather, OwnerAdminAcct: "weather-feed@weather-co", InvitedOrg: orgEnterprise, InvitedAdmin: "pulse-agent@enterprise-client"},
	{Name: "signals-channel", OwnerOrg: orgSignals, OwnerAdminAcct: "search-trends@signals-co", InvitedOrg: orgEnterprise, InvitedAdmin: "pulse-agent@enterprise-client"},
	{Name: "analytics-channel", OwnerOrg: orgAnalytics, OwnerAdminAcct: "correlator@analytics-co", InvitedOrg: orgEnterprise, InvitedAdmin: "pulse-agent@enterprise-client"},
}

type intraOrgWorkgroup struct {
	Name      string
	Org       string
	AdminAcct string
}

var intraOrgWorkgroups = []intraOrgWorkgroup{
	{Name: "markets-internal", Org: orgMarkets, AdminAcct: "equity-feed@markets-co"},
	{Name: "weather-internal", Org: orgWeather, AdminAcct: "weather-feed@weather-co"},
	{Name: "signals-internal", Org: orgSignals, AdminAcct: "search-trends@signals-co"},
}

// extraMembers maps each workgroup name to the agent emails that need
// to be added as members beyond the InitialAdminAccount that the
// workgroup is created with. The admin account is implicitly a member
// already; the listed emails are added via AddWorkgroupMember calls
// authenticated with the admin's account token.
var extraMembers = map[string][]string{
	"markets-internal":  {"fx-feed@markets-co", "commodities-feed@markets-co"},
	"signals-internal":  {"news-pulse@signals-co"},
	"markets-channel":   {"fx-feed@markets-co", "commodities-feed@markets-co"},
	"signals-channel":   {"news-pulse@signals-co"},
	"analytics-channel": {"narrator@analytics-co"},
}

// adminEmailByWorkgroup maps each workgroup name to the email of the
// account that owns it (the InitialAdminAccount the workgroup was
// created with). That account's token is used to authenticate
// AddWorkgroupMember calls.
var adminEmailByWorkgroup = map[string]string{
	"markets-internal":  "equity-feed@markets-co",
	"weather-internal":  "weather-feed@weather-co",
	"signals-internal":  "search-trends@signals-co",
	"markets-channel":   "equity-feed@markets-co",
	"weather-channel":   "weather-feed@weather-co",
	"signals-channel":   "search-trends@signals-co",
	"analytics-channel": "correlator@analytics-co",
}

func main() {
	controllerURL := flag.String("controller", "http://127.0.0.1:8080", "Controller URL")
	flag.Parse()

	adminToken := os.Getenv("AGORA_ADMIN_TOKEN")
	if adminToken == "" {
		fmt.Fprintln(os.Stderr, "AGORA_ADMIN_TOKEN must be set")
		os.Exit(1)
	}

	ctx := context.Background()
	client, err := api.NewClient(strings.TrimRight(*controllerURL, "/")+"/v1", staticAdmin{token: adminToken}, api.WithClient(http.DefaultClient))
	if err != nil {
		fmt.Fprintln(os.Stderr, "build api client:", err)
		os.Exit(1)
	}

	orgIDs := map[string]string{}
	for _, name := range []string{orgMarkets, orgWeather, orgSignals, orgAnalytics, orgEnterprise} {
		id, err := ensureOrg(ctx, client, name)
		if err != nil {
			fmt.Fprintln(os.Stderr, "ensure org", name, ":", err)
			os.Exit(1)
		}
		orgIDs[name] = id
		fmt.Printf("org   %-20s id=%s\n", name, id)
	}

	type seededAccount struct {
		Email string
		ID    string
		Token string
		Org   string
	}
	accounts := []seededAccount{}
	for _, orgName := range []string{orgMarkets, orgWeather, orgSignals, orgAnalytics, orgEnterprise} {
		for _, email := range agentsByOrg[orgName] {
			acctID, token, err := ensureAccount(ctx, client, orgIDs[orgName], email)
			if err != nil {
				fmt.Fprintln(os.Stderr, "ensure account", email, ":", err)
				os.Exit(1)
			}
			accounts = append(accounts, seededAccount{Email: email, ID: acctID, Token: token, Org: orgName})
			tokenLabel := "(existing)"
			if token != "" {
				tokenLabel = token
			}
			fmt.Printf("acct  %-30s org=%s id=%s token=%s\n", email, orgName, acctID, tokenLabel)
		}
	}

	accountByEmail := map[string]string{}
	for _, a := range accounts {
		accountByEmail[a.Email] = a.ID
	}

	for _, wg := range intraOrgWorkgroups {
		wgID, err := ensureIntraOrgWorkgroup(ctx, client, wg.Name, orgIDs[wg.Org], accountByEmail[wg.AdminAcct])
		if err != nil {
			fmt.Fprintln(os.Stderr, "ensure intra-org wg", wg.Name, ":", err)
			os.Exit(1)
		}
		fmt.Printf("wg    %-20s scope=intra-org owner=%s id=%s\n", wg.Name, wg.Org, wgID)
	}

	for _, ch := range interOrgChannels {
		wgID, err := ensureInterOrgWorkgroup(ctx, client, ch.Name, orgIDs[ch.OwnerOrg], accountByEmail[ch.OwnerAdminAcct], orgIDs[ch.InvitedOrg])
		if err != nil {
			fmt.Fprintln(os.Stderr, "ensure inter-org wg", ch.Name, ":", err)
			os.Exit(1)
		}
		fmt.Printf("wg    %-20s scope=inter-org owner=%s invited=%s id=%s\n", ch.Name, ch.OwnerOrg, ch.InvitedOrg, wgID)
		if err := acceptInvitation(ctx, client, wgID, orgIDs[ch.InvitedOrg], accountByEmail[ch.InvitedAdmin]); err != nil {
			fmt.Fprintln(os.Stderr, "accept invitation", ch.Name, ":", err)
			os.Exit(1)
		}
		fmt.Printf("inv   %-20s accepted by %s\n", ch.Name, ch.InvitedAdmin)
	}

	tokenByEmail := map[string]string{}
	for _, a := range accounts {
		if a.Token != "" {
			tokenByEmail[a.Email] = a.Token
		}
	}
	for _, wgName := range workgroupNames() {
		extras, ok := extraMembers[wgName]
		if !ok {
			continue
		}
		adminEmail := adminEmailByWorkgroup[wgName]
		adminToken := tokenByEmail[adminEmail]
		if adminToken == "" {
			fmt.Fprintf(os.Stderr, "skip member-add for %s: admin %s token not in this run\n", wgName, adminEmail)
			continue
		}
		adminClient, err := api.NewClient(strings.TrimRight(*controllerURL, "/")+"/v1", staticAccount{token: adminToken}, api.WithClient(http.DefaultClient))
		if err != nil {
			fmt.Fprintln(os.Stderr, "build admin-of-wg client:", err)
			os.Exit(1)
		}
		wgID, err := lookupWorkgroupIDForAccount(ctx, adminClient, wgName)
		if err != nil {
			fmt.Fprintln(os.Stderr, "lookup workgroup", wgName, ":", err)
			os.Exit(1)
		}
		for _, memberEmail := range extras {
			if err := addMember(ctx, adminClient, wgID, memberEmail); err != nil {
				fmt.Fprintln(os.Stderr, "add member", memberEmail, "->", wgName, ":", err)
				os.Exit(1)
			}
			fmt.Printf("memb  %-20s + %s\n", wgName, memberEmail)
		}
	}

	fmt.Println()
	fmt.Println("bootstrap complete.")
	fmt.Println("agent account tokens (distribute these to the corresponding environment roots):")
	for _, a := range accounts {
		token := a.Token
		if token == "" {
			token = "(existing — token not retrievable; rotate via 'agora account regenerate-token' if needed)"
		}
		fmt.Printf("  %-30s %s\n", a.Email, token)
	}
}

func workgroupNames() []string {
	names := make([]string, 0, len(intraOrgWorkgroups)+len(interOrgChannels))
	for _, wg := range intraOrgWorkgroups {
		names = append(names, wg.Name)
	}
	for _, ch := range interOrgChannels {
		names = append(names, ch.Name)
	}
	return names
}

type staticAccount struct{ token string }

func (s staticAccount) AccountTokenAuth(context.Context, api.OperationName) (api.AccountTokenAuth, error) {
	return api.AccountTokenAuth{APIKey: s.token}, nil
}
func (s staticAccount) AdminTokenAuth(context.Context, api.OperationName) (api.AdminTokenAuth, error) {
	return api.AdminTokenAuth{}, nil
}

func lookupWorkgroupIDForAccount(ctx context.Context, c *api.Client, name string) (string, error) {
	res, err := c.ListWorkgroups(ctx)
	if err != nil {
		return "", err
	}
	listing, ok := res.(*api.ListWorkgroupsResponse)
	if !ok {
		return "", fmt.Errorf("unexpected list workgroups response: %T", res)
	}
	for _, wg := range *listing {
		if strings.EqualFold(wg.Name, name) {
			return wg.ID, nil
		}
	}
	return "", fmt.Errorf("workgroup %q not visible to admin account", name)
}

func addMember(ctx context.Context, c *api.Client, wgID, email string) error {
	res, err := c.AddWorkgroupMember(ctx, &api.AddWorkgroupMemberRequest{AccountEmail: email}, api.AddWorkgroupMemberParams{WorkgroupId: wgID})
	if err != nil {
		return err
	}
	switch typed := res.(type) {
	case *api.WorkgroupMembership:
		return nil
	case *api.AddWorkgroupMemberConflict:
		return nil // already a member
	case *api.AddWorkgroupMemberBadRequest:
		return fmt.Errorf("bad request: %s", typed.Message)
	case *api.AddWorkgroupMemberNotFound:
		return fmt.Errorf("not found: %s", typed.Message)
	case *api.AddWorkgroupMemberForbidden:
		return fmt.Errorf("forbidden: %s", typed.Message)
	default:
		return fmt.Errorf("unexpected add member response: %T", res)
	}
}

func ensureOrg(ctx context.Context, client *api.Client, name string) (string, error) {
	res, err := client.CreateOrganization(ctx, &api.CreateOrganizationRequest{Name: name})
	if err != nil {
		return "", err
	}
	switch res.(type) {
	case *api.Organization:
		return res.(*api.Organization).ID, nil
	case *api.CreateOrganizationConflict:
		return findOrgIDByName(ctx, client, name)
	default:
		return "", fmt.Errorf("unexpected create organization response: %T", res)
	}
}

func findOrgIDByName(ctx context.Context, client *api.Client, name string) (string, error) {
	res, err := client.ListOrganizations(ctx)
	if err != nil {
		return "", err
	}
	listing, ok := res.(*api.ListOrganizationsResponse)
	if !ok {
		return "", fmt.Errorf("unexpected list orgs response: %T", res)
	}
	for _, org := range *listing {
		if strings.EqualFold(org.Name, name) {
			return org.ID, nil
		}
	}
	return "", fmt.Errorf("org %q not found", name)
}

func ensureAccount(ctx context.Context, client *api.Client, orgID, email string) (id, token string, err error) {
	res, err := client.CreateAccount(ctx, &api.CreateAccountRequest{Email: email, Password: "macro-pulse-bootstrap"}, api.CreateAccountParams{OrganizationId: orgID})
	if err != nil {
		return "", "", err
	}
	switch typed := res.(type) {
	case *api.AccountTokenResponse:
		acctID, lookupErr := findAccountIDByEmail(ctx, client, orgID, email)
		if lookupErr != nil {
			return "", typed.AccountToken, lookupErr
		}
		return acctID, typed.AccountToken, nil
	case *api.CreateAccountConflict:
		acctID, lookupErr := findAccountIDByEmail(ctx, client, orgID, email)
		return acctID, "", lookupErr
	default:
		return "", "", fmt.Errorf("unexpected create account response: %T", res)
	}
}

func findAccountIDByEmail(ctx context.Context, client *api.Client, orgID, email string) (string, error) {
	res, err := client.ListAccounts(ctx, api.ListAccountsParams{OrganizationId: orgID})
	if err != nil {
		return "", err
	}
	listing, ok := res.(*api.ListAccountsResponse)
	if !ok {
		return "", fmt.Errorf("unexpected list accounts response: %T", res)
	}
	for _, a := range *listing {
		if strings.EqualFold(a.Email, email) {
			return a.ID, nil
		}
	}
	return "", fmt.Errorf("account %q not found in org %s", email, orgID)
}

func ensureIntraOrgWorkgroup(ctx context.Context, client *api.Client, name, orgID, adminAcctID string) (string, error) {
	res, err := client.CreateWorkgroup(ctx, &api.CreateWorkgroupRequest{
		Name:                  name,
		Scope:                 api.CreateWorkgroupRequestScopeIntraOrg,
		OwnerOrganizationId:   orgID,
		InitialAdminAccountId: adminAcctID,
	})
	if err != nil {
		return "", err
	}
	switch typed := res.(type) {
	case *api.CreateWorkgroupResponse:
		return typed.Workgroup.ID, nil
	case *api.CreateWorkgroupConflict:
		return findWorkgroupIDByName(ctx, client, name, orgID)
	default:
		return "", fmt.Errorf("unexpected create workgroup response: %T", res)
	}
}

func ensureInterOrgWorkgroup(ctx context.Context, client *api.Client, name, ownerOrgID, ownerAdminID, invitedOrgID string) (string, error) {
	res, err := client.CreateWorkgroup(ctx, &api.CreateWorkgroupRequest{
		Name:                         name,
		Scope:                        api.CreateWorkgroupRequestScopeInterOrg,
		OwnerOrganizationId:          ownerOrgID,
		InitialAdminAccountId:        ownerAdminID,
		ParticipatingOrganizationIds: []string{invitedOrgID},
	})
	if err != nil {
		return "", err
	}
	switch typed := res.(type) {
	case *api.CreateWorkgroupResponse:
		return typed.Workgroup.ID, nil
	case *api.CreateWorkgroupConflict:
		return findWorkgroupIDByName(ctx, client, name, ownerOrgID)
	default:
		return "", fmt.Errorf("unexpected create workgroup response: %T", res)
	}
}

func findWorkgroupIDByName(ctx context.Context, client *api.Client, name, ownerOrgID string) (string, error) {
	params := api.ListAdminWorkgroupsParams{}
	params.OwnerOrganizationId.SetTo(ownerOrgID)
	res, err := client.ListAdminWorkgroups(ctx, params)
	if err != nil {
		return "", err
	}
	listing, ok := res.(*api.ListAdminWorkgroupsResponse)
	if !ok {
		return "", fmt.Errorf("unexpected list admin workgroups response: %T", res)
	}
	for _, entry := range *listing {
		if strings.EqualFold(entry.Workgroup.Name, name) {
			return entry.Workgroup.ID, nil
		}
	}
	return "", fmt.Errorf("workgroup %q not found in org %s", name, ownerOrgID)
}

func acceptInvitation(ctx context.Context, client *api.Client, wgID, orgID, adminAcctID string) error {
	res, err := client.AcceptWorkgroupInvitation(ctx,
		&api.AcceptWorkgroupInvitationRequest{InitialAdminAccountId: adminAcctID},
		api.AcceptWorkgroupInvitationParams{WorkgroupId: wgID, OrganizationId: orgID},
	)
	if err != nil {
		return err
	}
	switch typed := res.(type) {
	case *api.AcknowledgeWorkgroupInvitationResponse:
		return nil
	case *api.AcceptWorkgroupInvitationConflict:
		// Already acknowledged — idempotent success.
		return nil
	case *api.AcceptWorkgroupInvitationNotFound:
		return errors.New(typed.Message)
	default:
		return fmt.Errorf("unexpected accept invitation response: %T", res)
	}
}

type staticAdmin struct {
	token string
}

func (s staticAdmin) AccountTokenAuth(context.Context, api.OperationName) (api.AccountTokenAuth, error) {
	return api.AccountTokenAuth{}, nil
}

func (s staticAdmin) AdminTokenAuth(context.Context, api.OperationName) (api.AdminTokenAuth, error) {
	return api.AdminTokenAuth{APIKey: s.token}, nil
}
