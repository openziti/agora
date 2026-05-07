package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/michaelquigley/df/dd"
	"github.com/openziti/agora/environment"
	"github.com/openziti/agora/environment/env_core"
	"github.com/openziti/agora/internal/api"
)

const environmentIdentityName = "environment"

type agentConfig struct {
	Contract   string
	ContractID string
}

type gatewayIntegrationConfig struct {
	APIEndpoint       string
	EnvRoot           string
	AccountEmail      string
	AdvertisementName string
	ContractID        string
	ContractName      string
	WorkgroupID       string
	WorkgroupName     string
}

func enrollEnvironments(ctx context.Context, baseURL, apiEndpoint, demoRoot string, topo *topology, accounts map[string]seededAccount, contractIDs map[string]string) error {
	for _, spec := range topo.Accounts {
		if !spec.Env {
			continue
		}
		account := accounts[spec.Email]
		envRoot := filepath.Join(demoRoot, "envs", spec.Email)
		contractName := spec.Advertisement.Contract
		contractID := ""
		if contractName != "" {
			contractID = contractIDs[contractKey(spec.Email, contractName)]
		}

		if environmentFileExists(envRoot) {
			fmt.Printf("env   %-34s reuse %s\n", spec.Email, envRoot)
			if err := writeAgentConfig(envRoot, contractName, contractID); err != nil {
				return err
			}
			continue
		}
		if account.Token == "" {
			return fmt.Errorf("account %q token unavailable and env root is missing at %q; cannot enroll", spec.Email, envRoot)
		}
		if err := enrollEnvironment(ctx, baseURL, apiEndpoint, envRoot, account); err != nil {
			return fmt.Errorf("enroll environment for %q: %w", spec.Email, err)
		}
		if err := writeAgentConfig(envRoot, contractName, contractID); err != nil {
			return err
		}
		fmt.Printf("env   %-34s create %s\n", spec.Email, envRoot)
	}
	return nil
}

func enrollEnvironment(ctx context.Context, baseURL, apiEndpoint, envRoot string, account seededAccount) error {
	if err := os.MkdirAll(envRoot, 0o700); err != nil {
		return err
	}
	environment.SetRootDirName(envRoot)
	root, err := environment.LoadRoot()
	if err != nil {
		return err
	}
	if root.IsEnabled() {
		return nil
	}
	if err := root.SetConfig(&env_core.Config{APIEndpoint: apiEndpoint}); err != nil {
		return err
	}
	client, err := api.NewClient(baseURL, staticAccount{token: account.Token}, api.WithClient(http.DefaultClient))
	if err != nil {
		return err
	}
	host := account.Spec.Email
	description := account.Spec.DisplayName
	if description == "" {
		description = account.Spec.Email
	}
	req := &api.EnableEnvironmentRequest{}
	req.Host.SetTo(host)
	req.Description.SetTo(description)
	res, err := client.EnableEnvironment(ctx, req)
	if err != nil {
		return err
	}
	enabled, ok := res.(*api.EnableEnvironmentResponse)
	if !ok {
		return enableEnvironmentResponseError(res)
	}
	if err := root.SetEnvironment(&env_core.Environment{
		EnvironmentID: enabled.Environment.ID,
		AccountToken:  account.Token,
		ZitiIdentity:  enabled.Environment.ZitiIdentityId,
		APIEndpoint:   apiEndpoint,
	}); err != nil {
		return err
	}
	if err := root.SaveZitiIdentityNamed(environmentIdentityName, enabled.EnrollmentJson); err != nil {
		_ = root.DeleteEnvironment()
		return err
	}
	return nil
}

func enableEnvironmentResponseError(res api.EnableEnvironmentRes) error {
	switch typed := res.(type) {
	case *api.EnableEnvironmentUnauthorized:
		return errors.New(typed.Message)
	case *api.EnableEnvironmentConflict:
		return errors.New(typed.Message)
	case *api.EnableEnvironmentInternalServerError:
		return errors.New(typed.Message)
	default:
		return fmt.Errorf("unexpected enable environment response: %T", res)
	}
}

func environmentFileExists(envRoot string) bool {
	_, err := os.Stat(filepath.Join(envRoot, "environment.json"))
	return err == nil
}

func writeAgentConfig(envRoot, contractName, contractID string) error {
	if contractName == "" {
		return nil
	}
	if err := os.MkdirAll(envRoot, 0o700); err != nil {
		return err
	}
	path := filepath.Join(envRoot, "agent-config.yaml")
	if err := dd.UnbindYAMLFile(agentConfig{
		Contract:   contractName,
		ContractID: contractID,
	}, path, yamlFileOptions(0o600)); err != nil {
		return fmt.Errorf("write agent config %q: %w", path, err)
	}
	return nil
}

func writeGatewayConfigs(demoRoot, apiEndpoint string, topo *topology, accounts map[string]seededAccount, workgroupIDs map[string]string, contractIDs map[string]string) error {
	gatewayDir := filepath.Join(demoRoot, "gateways")
	if err := os.MkdirAll(gatewayDir, 0o700); err != nil {
		return err
	}
	workgroupID, ok := workgroupIDs["gateway-services"]
	if !ok {
		return errors.New("gateway-services workgroup was not provisioned")
	}
	for _, gateway := range topo.Gateways {
		account, ok := accounts[gateway.AccountEmail]
		if !ok {
			return fmt.Errorf("gateway %q references unknown account %q", gateway.Name, gateway.AccountEmail)
		}
		contractName := account.Spec.Advertisement.Contract
		contractID := contractIDs[contractKey(gateway.AccountEmail, contractName)]
		if contractID == "" {
			return fmt.Errorf("gateway %q has no contract id for %q", gateway.Name, contractName)
		}
		envRoot := filepath.Join(demoRoot, "envs", gateway.AccountEmail)
		path := filepath.Join(gatewayDir, gateway.Name+".yaml")
		cfg := gatewayIntegrationConfig{
			APIEndpoint:       apiEndpoint,
			EnvRoot:           envRoot,
			AccountEmail:      gateway.AccountEmail,
			AdvertisementName: gateway.AdvertisementName,
			ContractID:        contractID,
			ContractName:      contractName,
			WorkgroupID:       workgroupID,
			WorkgroupName:     "gateway-services",
		}
		if err := dd.UnbindYAMLFile(cfg, path, yamlFileOptions(0o600)); err != nil {
			return fmt.Errorf("write gateway config %q: %w", path, err)
		}
		fmt.Printf("gate  %-24s config=%s\n", gateway.Name, path)
	}
	return nil
}

func yamlFileOptions(mode os.FileMode) *dd.Options {
	return &dd.Options{
		File: &dd.FileOptions{
			Mode: &mode,
		},
	}
}
