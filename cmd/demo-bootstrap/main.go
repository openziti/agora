package main

import (
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/michaelquigley/df/dd"
	"github.com/openziti/agora/internal/api"
)

const (
	defaultControllerURL = "http://127.0.0.1:8080"
	defaultDemoRootName  = ".agora-demo"
	defaultSeedHistory   = 7 * 24 * time.Hour
)

//go:embed topology.yaml
var defaultTopologyYAML []byte

type topology struct {
	Organizations []organizationSpec
	Contracts     []contractSpec
	Accounts      []accountSpec
	Workgroups    []workgroupSpec
	Gateways      []gatewaySpec
}

type organizationSpec struct {
	Name string
}

type accountSpec struct {
	Email         string
	DisplayName   string
	Organization  string
	Password      string
	Role          string
	Status        string
	Env           bool
	Gateway       string
	Advertisement advertisementSpec
}

type advertisementSpec struct {
	Name      string
	Workgroup string
	Contract  string
}

type workgroupSpec struct {
	Name                 string
	Description          string
	Scope                string
	OwnerOrganization    string
	InitialAdmin         string
	InvitedOrganizations []workgroupInvitationSpec
	Members              []workgroupMemberSpec
}

type workgroupInvitationSpec struct {
	Organization string
	InitialAdmin string
}

type workgroupMemberSpec struct {
	Email string
	Role  string
}

type contractSpec struct {
	Name                         string
	Description                  string
	MaxDurationSeconds           int
	MaxEnvelopeCount             int
	MaxEnvelopeBytes             int
	AllowedMessageTypes          []string
	RequiredWorkgroupMemberships []string
	MinAccountAgeDays            int
	AccessMode                   string
}

type gatewaySpec struct {
	Name              string
	AccountEmail      string
	AdvertisementName string
}

type seededAccount struct {
	Spec  accountSpec
	ID    string
	Token string
}

type durationFlag struct {
	value time.Duration
}

func (f *durationFlag) String() string {
	if f == nil {
		return ""
	}
	if f.value%(24*time.Hour) == 0 {
		return fmt.Sprintf("%dd", int(f.value/(24*time.Hour)))
	}
	return f.value.String()
}

func (f *durationFlag) Set(raw string) error {
	d, err := parseDemoDuration(raw)
	if err != nil {
		return err
	}
	f.value = d
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "demo-bootstrap:", err)
		os.Exit(1)
	}
}

func run() error {
	var controllerURL string
	var adminToken string
	var topologyPath string
	var resetEnvs bool
	seedHistory := durationFlag{value: defaultSeedHistory}

	flag.StringVar(&controllerURL, "controller", defaultControllerURL, "Controller URL")
	flag.StringVar(&adminToken, "admin-token", os.Getenv("AGORA_ADMIN_TOKEN"), "Admin token; defaults to AGORA_ADMIN_TOKEN")
	flag.StringVar(&topologyPath, "topology", "", "Optional topology YAML path; overlays the embedded topology")
	flag.Var(&seedHistory, "seed-history", "Synthetic history duration; supports Go durations and day suffixes like 7d")
	flag.BoolVar(&resetEnvs, "reset-envs", false, "Delete and recreate the demo env root tree before enrollment")
	flag.Parse()

	if adminToken == "" {
		return errors.New("admin token is required; set AGORA_ADMIN_TOKEN or pass --admin-token")
	}

	controllerURL = strings.TrimRight(controllerURL, "/")
	baseURL := controllerURL + "/v1"
	demoRoot, err := demoRoot()
	if err != nil {
		return err
	}
	topo, err := loadTopology(topologyPath)
	if err != nil {
		return err
	}

	ctx := context.Background()
	adminClient, err := api.NewClient(baseURL, staticAdmin{token: adminToken}, api.WithClient(http.DefaultClient))
	if err != nil {
		return fmt.Errorf("build admin api client: %w", err)
	}
	loginClient, err := api.NewClient(baseURL, noAuth{}, api.WithClient(http.DefaultClient))
	if err != nil {
		return fmt.Errorf("build login api client: %w", err)
	}

	if resetEnvs {
		envTree := filepath.Join(demoRoot, "envs")
		if err := os.RemoveAll(envTree); err != nil {
			return fmt.Errorf("reset envs tree %q: %w", envTree, err)
		}
		fmt.Printf("envs  reset %s\n", envTree)
	}

	orgIDs, err := provisionOrganizations(ctx, adminClient, topo)
	if err != nil {
		return err
	}
	accounts, err := provisionAccounts(ctx, adminClient, loginClient, topo, orgIDs)
	if err != nil {
		return err
	}
	workgroupIDs, err := provisionWorkgroups(ctx, baseURL, adminClient, topo, orgIDs, accounts)
	if err != nil {
		return err
	}
	contractIDs, err := provisionContracts(ctx, baseURL, topo, accounts, workgroupIDs)
	if err != nil {
		return err
	}
	if err := enrollEnvironments(ctx, baseURL, controllerURL, demoRoot, topo, accounts, contractIDs); err != nil {
		return err
	}
	if err := writeGatewayConfigs(demoRoot, controllerURL, topo, accounts, workgroupIDs, contractIDs); err != nil {
		return err
	}
	seedPath := filepath.Join(demoRoot, "seed-history.sql")
	rows, err := synthesizeHistory(seedPath, seedHistory.value, topo, orgIDs, accounts, workgroupIDs, contractIDs)
	if err != nil {
		return err
	}
	fmt.Printf("seed  %s rows=%d window=%s\n", seedPath, rows, seedHistory.String())

	fmt.Println("done  demo bootstrap complete")
	fmt.Println("login demo@agora.local / Agora-Demo-1")
	return nil
}

func loadTopology(path string) (*topology, error) {
	topo := &topology{}
	if err := dd.MergeYAML(topo, defaultTopologyYAML); err != nil {
		return nil, fmt.Errorf("load embedded topology: %w", err)
	}
	if path != "" {
		if err := dd.MergeYAMLFile(topo, path); err != nil {
			return nil, fmt.Errorf("load topology %q: %w", path, err)
		}
	}
	if err := topo.validate(); err != nil {
		return nil, err
	}
	return topo, nil
}

func (t *topology) validate() error {
	if len(t.Organizations) == 0 {
		return errors.New("topology has no organizations")
	}
	contracts := map[string]struct{}{}
	for _, c := range t.Contracts {
		if c.Name == "" {
			return errors.New("topology contract missing name")
		}
		contracts[strings.ToLower(c.Name)] = struct{}{}
	}
	for _, acct := range t.Accounts {
		if acct.Email == "" || acct.Organization == "" {
			return fmt.Errorf("topology account requires email and organization: %#v", acct)
		}
		if acct.Advertisement.Contract != "" {
			if _, ok := contracts[strings.ToLower(acct.Advertisement.Contract)]; !ok {
				return fmt.Errorf("account %q references unknown contract %q", acct.Email, acct.Advertisement.Contract)
			}
		}
	}
	for _, wg := range t.Workgroups {
		if wg.Name == "" || wg.Scope == "" || wg.OwnerOrganization == "" || wg.InitialAdmin == "" {
			return fmt.Errorf("topology workgroup requires name, scope, owner_organization, and initial_admin: %#v", wg)
		}
	}
	return nil
}

func provisionOrganizations(ctx context.Context, client *api.Client, topo *topology) (map[string]string, error) {
	orgIDs := map[string]string{}
	for _, org := range topo.Organizations {
		id, err := ensureOrg(ctx, client, org.Name)
		if err != nil {
			return nil, fmt.Errorf("ensure org %q: %w", org.Name, err)
		}
		orgIDs[org.Name] = id
		fmt.Printf("org   %-24s id=%s\n", org.Name, id)
	}
	return orgIDs, nil
}

func provisionAccounts(ctx context.Context, adminClient, loginClient *api.Client, topo *topology, orgIDs map[string]string) (map[string]seededAccount, error) {
	accounts := map[string]seededAccount{}
	for _, acct := range topo.Accounts {
		orgID, ok := orgIDs[acct.Organization]
		if !ok {
			return nil, fmt.Errorf("account %q references unknown org %q", acct.Email, acct.Organization)
		}
		id, token, err := ensureAccount(ctx, adminClient, loginClient, orgID, acct)
		if err != nil {
			return nil, fmt.Errorf("ensure account %q: %w", acct.Email, err)
		}
		accounts[acct.Email] = seededAccount{Spec: acct, ID: id, Token: token}
		tokenLabel := "(existing; token unavailable)"
		if token != "" {
			tokenLabel = token
		}
		fmt.Printf("acct  %-34s org=%s id=%s token=%s\n", acct.Email, acct.Organization, id, tokenLabel)
	}
	return accounts, nil
}

func provisionWorkgroups(ctx context.Context, baseURL string, adminClient *api.Client, topo *topology, orgIDs map[string]string, accounts map[string]seededAccount) (map[string]string, error) {
	workgroupIDs := map[string]string{}
	for _, wg := range topo.Workgroups {
		id, err := ensureWorkgroup(ctx, adminClient, wg, orgIDs, accounts)
		if err != nil {
			return nil, fmt.Errorf("ensure workgroup %q: %w", wg.Name, err)
		}
		workgroupIDs[wg.Name] = id
		fmt.Printf("wg    %-24s scope=%s id=%s\n", wg.Name, wg.Scope, id)
		for _, invitation := range wg.InvitedOrganizations {
			orgID, ok := orgIDs[invitation.Organization]
			if !ok {
				return nil, fmt.Errorf("workgroup %q invitation references unknown org %q", wg.Name, invitation.Organization)
			}
			admin, ok := accounts[invitation.InitialAdmin]
			if !ok {
				return nil, fmt.Errorf("workgroup %q invitation references unknown admin %q", wg.Name, invitation.InitialAdmin)
			}
			if err := acceptInvitation(ctx, adminClient, id, orgID, admin.ID); err != nil {
				return nil, fmt.Errorf("accept invitation %q for %q: %w", wg.Name, invitation.Organization, err)
			}
			fmt.Printf("inv   %-24s accepted_by=%s\n", wg.Name, invitation.InitialAdmin)
		}
	}

	for _, wg := range topo.Workgroups {
		if len(wg.Members) == 0 {
			continue
		}
		admin, ok := accounts[wg.InitialAdmin]
		if !ok {
			return nil, fmt.Errorf("workgroup %q references unknown initial admin %q", wg.Name, wg.InitialAdmin)
		}
		if admin.Token == "" {
			return nil, fmt.Errorf("account %q token unavailable; cannot add members to workgroup %q", wg.InitialAdmin, wg.Name)
		}
		client, err := accountClient(baseURL, admin.Token)
		if err != nil {
			return nil, err
		}
		for _, member := range wg.Members {
			if err := addMember(ctx, client, workgroupIDs[wg.Name], member); err != nil {
				return nil, fmt.Errorf("add member %q to workgroup %q: %w", member.Email, wg.Name, err)
			}
			fmt.Printf("memb  %-24s + %s\n", wg.Name, member.Email)
		}
	}

	return workgroupIDs, nil
}

func provisionContracts(ctx context.Context, baseURL string, topo *topology, accounts map[string]seededAccount, workgroupIDs map[string]string) (map[string]string, error) {
	contractsByName := map[string]contractSpec{}
	for _, spec := range topo.Contracts {
		contractsByName[strings.ToLower(spec.Name)] = spec
	}

	contractIDs := map[string]string{}
	for _, acct := range topo.Accounts {
		contractName := acct.Advertisement.Contract
		if contractName == "" {
			continue
		}
		spec, ok := contractsByName[strings.ToLower(contractName)]
		if !ok {
			return nil, fmt.Errorf("account %q references unknown contract %q", acct.Email, contractName)
		}
		seeded := accounts[acct.Email]
		if seeded.Token == "" {
			return nil, fmt.Errorf("account %q token unavailable; cannot ensure contract %q", acct.Email, contractName)
		}
		client, err := accountClient(baseURL, seeded.Token)
		if err != nil {
			return nil, err
		}
		contractID, err := ensureContract(ctx, client, spec, workgroupIDs)
		if err != nil {
			return nil, fmt.Errorf("ensure contract %q for %q: %w", contractName, acct.Email, err)
		}
		contractIDs[contractKey(acct.Email, contractName)] = contractID
		fmt.Printf("con   %-30s owner=%-34s id=%s\n", contractName, acct.Email, contractID)
	}
	return contractIDs, nil
}

func ensureOrg(ctx context.Context, client *api.Client, name string) (string, error) {
	res, err := client.CreateOrganization(ctx, &api.CreateOrganizationRequest{Name: name})
	if err != nil {
		return "", err
	}
	switch typed := res.(type) {
	case *api.Organization:
		return typed.ID, nil
	case *api.CreateOrganizationConflict:
		return findOrgIDByName(ctx, client, name)
	case *api.CreateOrganizationUnauthorized:
		return "", errors.New(typed.Message)
	case *api.CreateOrganizationInternalServerError:
		return "", errors.New(typed.Message)
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
		return "", fmt.Errorf("unexpected list organizations response: %T", res)
	}
	for _, org := range *listing {
		if strings.EqualFold(org.Name, name) {
			return org.ID, nil
		}
	}
	return "", fmt.Errorf("organization %q not found", name)
}

func ensureAccount(ctx context.Context, adminClient, loginClient *api.Client, orgID string, spec accountSpec) (id, token string, err error) {
	req, err := createAccountRequest(spec)
	if err != nil {
		return "", "", err
	}

	res, err := adminClient.CreateAccount(ctx, req, api.CreateAccountParams{OrganizationId: orgID})
	if err != nil {
		return "", "", err
	}
	switch typed := res.(type) {
	case *api.AccountTokenResponse:
		acctID, lookupErr := findAccountIDByEmail(ctx, adminClient, orgID, spec.Email)
		if lookupErr != nil {
			return "", typed.AccountToken, lookupErr
		}
		return acctID, typed.AccountToken, nil
	case *api.CreateAccountConflict:
		acctID, lookupErr := findAccountIDByEmail(ctx, adminClient, orgID, spec.Email)
		if lookupErr != nil {
			return "", "", lookupErr
		}
		token, loginErr := loginAccount(ctx, loginClient, spec.Email, accountPassword(spec))
		if loginErr != nil {
			return acctID, "", nil
		}
		return acctID, token, nil
	case *api.CreateAccountNotFound:
		return "", "", errors.New(typed.Message)
	case *api.CreateAccountUnauthorized:
		return "", "", errors.New(typed.Message)
	case *api.CreateAccountInternalServerError:
		return "", "", errors.New(typed.Message)
	default:
		return "", "", fmt.Errorf("unexpected create account response: %T", res)
	}
}

func createAccountRequest(spec accountSpec) (*api.CreateAccountRequest, error) {
	req := &api.CreateAccountRequest{
		Email:    spec.Email,
		Password: accountPassword(spec),
	}
	if spec.DisplayName != "" {
		req.DisplayName.SetTo(spec.DisplayName)
	}
	if spec.Role != "" {
		role, err := apiAccountRole(spec.Role)
		if err != nil {
			return nil, err
		}
		req.Role.SetTo(role)
	}
	if spec.Status != "" {
		status, err := apiAccountStatus(spec.Status)
		if err != nil {
			return nil, err
		}
		req.Status.SetTo(status)
	}
	return req, nil
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
	for _, acct := range *listing {
		if strings.EqualFold(acct.Email, email) {
			return acct.ID, nil
		}
	}
	return "", fmt.Errorf("account %q not found in org %s", email, orgID)
}

func loginAccount(ctx context.Context, client *api.Client, email, password string) (string, error) {
	res, err := client.Login(ctx, &api.LoginRequest{Email: email, Password: password})
	if err != nil {
		return "", err
	}
	switch typed := res.(type) {
	case *api.AccountTokenResponse:
		return typed.AccountToken, nil
	case *api.LoginUnauthorized:
		return "", errors.New(typed.Message)
	case *api.LoginInternalServerError:
		return "", errors.New(typed.Message)
	default:
		return "", fmt.Errorf("unexpected login response: %T", res)
	}
}

func ensureWorkgroup(ctx context.Context, client *api.Client, spec workgroupSpec, orgIDs map[string]string, accounts map[string]seededAccount) (string, error) {
	ownerOrgID, ok := orgIDs[spec.OwnerOrganization]
	if !ok {
		return "", fmt.Errorf("unknown owner org %q", spec.OwnerOrganization)
	}
	admin, ok := accounts[spec.InitialAdmin]
	if !ok {
		return "", fmt.Errorf("unknown initial admin %q", spec.InitialAdmin)
	}
	scope, err := apiWorkgroupScope(spec.Scope)
	if err != nil {
		return "", err
	}
	req := &api.CreateWorkgroupRequest{
		Name:                  spec.Name,
		Scope:                 scope,
		OwnerOrganizationId:   ownerOrgID,
		InitialAdminAccountId: admin.ID,
	}
	if spec.Description != "" {
		req.Description.SetTo(spec.Description)
	}
	for _, invitation := range spec.InvitedOrganizations {
		orgID, ok := orgIDs[invitation.Organization]
		if !ok {
			return "", fmt.Errorf("unknown invited org %q", invitation.Organization)
		}
		req.ParticipatingOrganizationIds = append(req.ParticipatingOrganizationIds, orgID)
	}

	res, err := client.CreateWorkgroup(ctx, req)
	if err != nil {
		return "", err
	}
	switch typed := res.(type) {
	case *api.CreateWorkgroupResponse:
		return typed.Workgroup.ID, nil
	case *api.CreateWorkgroupConflict:
		return findWorkgroupIDByName(ctx, client, spec.Name, ownerOrgID)
	case *api.CreateWorkgroupBadRequest:
		return "", errors.New(typed.Message)
	case *api.CreateWorkgroupUnauthorized:
		return "", errors.New(typed.Message)
	case *api.CreateWorkgroupInternalServerError:
		return "", errors.New(typed.Message)
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

func acceptInvitation(ctx context.Context, client *api.Client, workgroupID, orgID, adminAccountID string) error {
	res, err := client.AcceptWorkgroupInvitation(ctx,
		&api.AcceptWorkgroupInvitationRequest{InitialAdminAccountId: adminAccountID},
		api.AcceptWorkgroupInvitationParams{WorkgroupId: workgroupID, OrganizationId: orgID},
	)
	if err != nil {
		return err
	}
	switch typed := res.(type) {
	case *api.AcknowledgeWorkgroupInvitationResponse:
		return nil
	case *api.AcceptWorkgroupInvitationConflict:
		return nil
	case *api.AcceptWorkgroupInvitationNotFound:
		return errors.New(typed.Message)
	case *api.AcceptWorkgroupInvitationUnauthorized:
		return errors.New(typed.Message)
	case *api.AcceptWorkgroupInvitationInternalServerError:
		return errors.New(typed.Message)
	default:
		return fmt.Errorf("unexpected accept invitation response: %T", res)
	}
}

func addMember(ctx context.Context, client *api.Client, workgroupID string, member workgroupMemberSpec) error {
	req := &api.AddWorkgroupMemberRequest{AccountEmail: member.Email}
	if member.Role != "" {
		role, err := apiWorkgroupMemberRole(member.Role)
		if err != nil {
			return err
		}
		req.Role.SetTo(role)
	}
	res, err := client.AddWorkgroupMember(ctx, req, api.AddWorkgroupMemberParams{WorkgroupId: workgroupID})
	if err != nil {
		return err
	}
	switch typed := res.(type) {
	case *api.WorkgroupMembership:
		return nil
	case *api.AddWorkgroupMemberConflict:
		return nil
	case *api.AddWorkgroupMemberBadRequest:
		return errors.New(typed.Message)
	case *api.AddWorkgroupMemberForbidden:
		return errors.New(typed.Message)
	case *api.AddWorkgroupMemberNotFound:
		return errors.New(typed.Message)
	case *api.AddWorkgroupMemberUnauthorized:
		return errors.New(typed.Message)
	case *api.AddWorkgroupMemberInternalServerError:
		return errors.New(typed.Message)
	default:
		return fmt.Errorf("unexpected add member response: %T", res)
	}
}

func ensureContract(ctx context.Context, client *api.Client, spec contractSpec, workgroupIDs map[string]string) (string, error) {
	res, err := client.ListContracts(ctx)
	if err != nil {
		return "", err
	}
	listing, ok := res.(*api.ListContractsResponse)
	if !ok {
		return "", fmt.Errorf("unexpected list contracts response: %T", res)
	}
	for _, contract := range *listing {
		if strings.EqualFold(contract.Name, spec.Name) {
			return contract.ID, nil
		}
	}

	req, err := createContractRequest(spec, workgroupIDs)
	if err != nil {
		return "", err
	}
	created, err := client.CreateContract(ctx, req)
	if err != nil {
		return "", err
	}
	switch typed := created.(type) {
	case *api.Contract:
		return typed.ID, nil
	case *api.CreateContractConflict:
		return findContractIDByName(ctx, client, spec.Name)
	case *api.CreateContractBadRequest:
		return "", errors.New(typed.Message)
	case *api.CreateContractForbidden:
		return "", errors.New(typed.Message)
	case *api.CreateContractUnauthorized:
		return "", errors.New(typed.Message)
	case *api.CreateContractInternalServerError:
		return "", errors.New(typed.Message)
	default:
		return "", fmt.Errorf("unexpected create contract response: %T", created)
	}
}

func findContractIDByName(ctx context.Context, client *api.Client, name string) (string, error) {
	res, err := client.ListContracts(ctx)
	if err != nil {
		return "", err
	}
	listing, ok := res.(*api.ListContractsResponse)
	if !ok {
		return "", fmt.Errorf("unexpected list contracts response: %T", res)
	}
	for _, contract := range *listing {
		if strings.EqualFold(contract.Name, name) {
			return contract.ID, nil
		}
	}
	return "", fmt.Errorf("contract %q not found", name)
}

func createContractRequest(spec contractSpec, workgroupIDs map[string]string) (*api.CreateContractRequest, error) {
	req := &api.CreateContractRequest{Name: spec.Name}
	if spec.Description != "" {
		req.Description.SetTo(spec.Description)
	}
	if spec.MaxDurationSeconds > 0 {
		req.MaxDurationSeconds.SetTo(spec.MaxDurationSeconds)
	}
	if spec.MaxEnvelopeCount > 0 {
		req.MaxEnvelopeCount.SetTo(spec.MaxEnvelopeCount)
	}
	if spec.MaxEnvelopeBytes > 0 {
		req.MaxEnvelopeBytes.SetTo(spec.MaxEnvelopeBytes)
	}
	if len(spec.AllowedMessageTypes) > 0 {
		req.AllowedMessageTypes = append([]string(nil), spec.AllowedMessageTypes...)
	}
	for _, name := range spec.RequiredWorkgroupMemberships {
		id, ok := workgroupIDs[name]
		if !ok {
			return nil, fmt.Errorf("contract %q references unknown required workgroup %q", spec.Name, name)
		}
		req.RequiredWorkgroupMemberships = append(req.RequiredWorkgroupMemberships, id)
	}
	if spec.MinAccountAgeDays > 0 {
		req.MaturityRequirements.SetTo(api.MaturityRequirements{
			MinAccountAgeDays: api.NewOptInt(spec.MinAccountAgeDays),
		})
	}
	if spec.AccessMode != "" {
		mode, err := apiContractAccessMode(spec.AccessMode)
		if err != nil {
			return nil, err
		}
		req.AccessMode.SetTo(mode)
	}
	return req, nil
}

func accountClient(baseURL, token string) (*api.Client, error) {
	return api.NewClient(baseURL, staticAccount{token: token}, api.WithClient(http.DefaultClient))
}

func accountPassword(spec accountSpec) string {
	if spec.Password != "" {
		return spec.Password
	}
	return "macro-pulse-bootstrap"
}

func apiAccountRole(raw string) (api.CreateAccountRequestRole, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "admin":
		return api.CreateAccountRequestRoleAdmin, nil
	case "member":
		return api.CreateAccountRequestRoleMember, nil
	default:
		return "", fmt.Errorf("unknown account role %q", raw)
	}
}

func apiAccountStatus(raw string) (api.CreateAccountRequestStatus, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "active":
		return api.CreateAccountRequestStatusActive, nil
	case "disabled":
		return api.CreateAccountRequestStatusDisabled, nil
	default:
		return "", fmt.Errorf("unknown account status %q", raw)
	}
}

func apiWorkgroupScope(raw string) (api.CreateWorkgroupRequestScope, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(api.CreateWorkgroupRequestScopeInterOrg):
		return api.CreateWorkgroupRequestScopeInterOrg, nil
	case string(api.CreateWorkgroupRequestScopeIntraOrg):
		return api.CreateWorkgroupRequestScopeIntraOrg, nil
	default:
		return "", fmt.Errorf("unknown workgroup scope %q", raw)
	}
}

func apiWorkgroupMemberRole(raw string) (api.WorkgroupMembershipRole, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "admin":
		return api.WorkgroupMembershipRoleAdmin, nil
	case "", "member":
		return api.WorkgroupMembershipRoleMember, nil
	default:
		return "", fmt.Errorf("unknown workgroup role %q", raw)
	}
}

func apiContractAccessMode(raw string) (api.ContractAccessMode, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "open":
		return api.ContractAccessModeOpen, nil
	case "approval_required":
		return api.ContractAccessModeApprovalRequired, nil
	default:
		return "", fmt.Errorf("unknown contract access mode %q", raw)
	}
}

func contractKey(email, name string) string {
	return strings.ToLower(email) + "|" + strings.ToLower(name)
}

func demoRoot() (string, error) {
	if root := os.Getenv("AGORA_DEMO_ROOT"); root != "" {
		return expandUserPath(root)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, defaultDemoRootName), nil
}

func expandUserPath(path string) (string, error) {
	if path == "~" {
		return os.UserHomeDir()
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
	}
	return path, nil
}

func parseDemoDuration(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, errors.New("duration is required")
	}
	if raw == "0" {
		return 0, nil
	}
	if strings.HasSuffix(raw, "d") {
		days, err := time.ParseDuration(strings.TrimSuffix(raw, "d") + "h")
		if err != nil {
			return 0, err
		}
		return days * 24, nil
	}
	return time.ParseDuration(raw)
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

type staticAccount struct {
	token string
}

func (s staticAccount) AccountTokenAuth(context.Context, api.OperationName) (api.AccountTokenAuth, error) {
	return api.AccountTokenAuth{APIKey: s.token}, nil
}

func (s staticAccount) AdminTokenAuth(context.Context, api.OperationName) (api.AdminTokenAuth, error) {
	return api.AdminTokenAuth{}, nil
}

type noAuth struct{}

func (noAuth) AccountTokenAuth(context.Context, api.OperationName) (api.AccountTokenAuth, error) {
	return api.AccountTokenAuth{}, nil
}

func (noAuth) AdminTokenAuth(context.Context, api.OperationName) (api.AdminTokenAuth, error) {
	return api.AdminTokenAuth{}, nil
}
