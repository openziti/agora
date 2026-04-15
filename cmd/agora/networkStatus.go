package main

import (
	"fmt"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/openziti/agora/internal/clioutput"
	networkpb "github.com/openziti/agora/internal/network/agent/pb"
	"github.com/spf13/cobra"
)

func init() {
	networkCmd.AddCommand(newNetworkStatusCommand())
}

func newNetworkStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show local Layer 1 network runtime status",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			root := loadRoot()
			status, err := networkStatusOrNil(root)
			if err != nil {
				return err
			}
			if status == nil {
				fmt.Println("network: not running")
				return nil
			}

			fmt.Println()
			fmt.Printf("network: running\n")
			fmt.Printf("socket: %s\n", status.SocketPath)
			fmt.Printf("pid: %d\n", status.Pid)
			fmt.Printf("started: %s\n", clioutput.TimeUTC(status.StartedAt.AsTime()))
			if status.EnvironmentEnabled {
				fmt.Printf("environment: enabled '%s'\n", status.EnvironmentId)
			} else {
				fmt.Println("environment: disabled")
			}
			fmt.Println()

			if len(status.Serves) > 0 {
				t := clioutput.NewTable()
				t.AppendHeader(table.Row{"Serve", "Mode", "Backend", "State", "Tunnel ID"})
				for _, serve := range status.Serves {
					t.AppendRow(table.Row{
						serve.Desired.Name,
						serve.Desired.Mode,
						serve.Desired.BackendTarget,
						formatRuntimeState(serve.State),
						clioutput.StringOrDash(serve.TunnelId),
					})
				}
				clioutput.PrintTable(t)
			}

			if len(status.Connects) > 0 {
				t := clioutput.NewTable()
				t.AppendHeader(table.Row{"Connect", "Listen", "State", "Tunnel ID"})
				for _, connect := range status.Connects {
					t.AppendRow(table.Row{
						connect.Desired.Name,
						connect.Desired.ListenAddress,
						formatRuntimeState(connect.State),
						clioutput.StringOrDash(connect.TunnelId),
					})
				}
				clioutput.PrintTable(t)
			}

			if len(status.Serves) == 0 && len(status.Connects) == 0 {
				fmt.Println("no desired serves or connects configured")
			}

			return nil
		},
	}
}

func formatRuntimeState(state networkpb.RuntimeState) string {
	return strings.ToLower(strings.TrimPrefix(state.String(), "RUNTIME_STATE_"))
}
