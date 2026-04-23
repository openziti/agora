package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/openziti/agora/internal/api"
	"github.com/spf13/cobra"
)

var workgroupCmd = &cobra.Command{
	Use:   "workgroup",
	Short: "Layer 2 workgroup commands",
}

var workgroupMemberCmd = &cobra.Command{
	Use:   "member",
	Short: "Workgroup membership commands",
}

func init() {
	rootCmd.AddCommand(workgroupCmd)
	workgroupCmd.AddCommand(workgroupMemberCmd)
}

// resolveWorkgroupID resolves a name-or-id token to a wg_... ID by
// consulting the workgroups visible to the caller. If the token is
// already a wg_... ID, it is returned as-is. If multiple visible
// workgroups share the name, the call panics with a list of
// candidates so the caller can disambiguate by ID.
func resolveWorkgroupID(client *api.Client, token string) string {
	token = strings.TrimSpace(token)
	if strings.HasPrefix(token, "wg_") {
		return token
	}
	res, err := client.ListWorkgroups(context.Background())
	panicIfErr(err)
	listing, ok := res.(*api.ListWorkgroupsResponse)
	if !ok {
		panic(fmt.Sprintf("unexpected list workgroups response: %T", res))
	}
	matches := make([]api.Workgroup, 0)
	for _, wg := range *listing {
		if strings.EqualFold(wg.Name, token) {
			matches = append(matches, wg)
		}
	}
	switch len(matches) {
	case 0:
		panic(fmt.Sprintf("no workgroup matches name or id '%s'", token))
	case 1:
		return matches[0].ID
	default:
		ids := make([]string, len(matches))
		for i, wg := range matches {
			ids[i] = wg.ID
		}
		panic(fmt.Sprintf("multiple workgroups match name '%s'; specify the id explicitly: %s", token, strings.Join(ids, ", ")))
	}
}
