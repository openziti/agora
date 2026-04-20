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
			if heartbeat := status.EnvironmentHeartbeat; heartbeat != nil {
				fmt.Printf("heartbeat: %s\n", formatEnvironmentHeartbeatState(heartbeat.State))
				if heartbeat.LastAttemptAt != nil {
					fmt.Printf("heartbeat last attempt: %s\n", clioutput.TimeUTC(heartbeat.LastAttemptAt.AsTime()))
				}
				if heartbeat.LastSuccessAt != nil {
					fmt.Printf("heartbeat last success: %s\n", clioutput.TimeUTC(heartbeat.LastSuccessAt.AsTime()))
				}
				if heartbeat.LastError != "" {
					fmt.Printf("heartbeat last error: %s\n", heartbeat.LastError)
				}
			}
			fmt.Println()

			if len(status.Serves) > 0 {
				t := clioutput.NewTable()
				t.AppendHeader(table.Row{"Serve", "Mode", "Backend", "State", "Tunnel ID", "Serve ID", "Retry", "Next Retry", "Last Started", "Last Error"})
				for _, serve := range status.Serves {
					lastStarted := "-"
					if serve.LastStartedAt != nil {
						lastStarted = clioutput.TimeUTC(serve.LastStartedAt.AsTime())
					}
					nextRetry := "-"
					if serve.NextRetryAt != nil {
						nextRetry = clioutput.TimeUTC(serve.NextRetryAt.AsTime())
					}
					t.AppendRow(table.Row{
						serve.Desired.Name,
						serve.Desired.Mode,
						serve.Desired.BackendTarget,
						formatRuntimeState(serve.State),
						clioutput.StringOrDash(serve.TunnelId),
						clioutput.StringOrDash(serve.ServeId),
						serve.RetryAttempt,
						nextRetry,
						lastStarted,
						clioutput.StringOrDash(serve.LastError),
					})
				}
				clioutput.PrintTable(t)
			}

			if len(status.Connects) > 0 {
				t := clioutput.NewTable()
				t.AppendHeader(table.Row{"Connect", "Listen", "State", "Tunnel ID", "Attachment ID", "Retry", "Next Retry", "Last Started", "Last Error"})
				for _, connect := range status.Connects {
					lastStarted := "-"
					if connect.LastStartedAt != nil {
						lastStarted = clioutput.TimeUTC(connect.LastStartedAt.AsTime())
					}
					nextRetry := "-"
					if connect.NextRetryAt != nil {
						nextRetry = clioutput.TimeUTC(connect.NextRetryAt.AsTime())
					}
					t.AppendRow(table.Row{
						connect.Desired.Name,
						connect.Desired.ListenAddress,
						formatRuntimeState(connect.State),
						clioutput.StringOrDash(connect.TunnelId),
						clioutput.StringOrDash(connect.AttachmentId),
						connect.RetryAttempt,
						nextRetry,
						lastStarted,
						clioutput.StringOrDash(connect.LastError),
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

func formatEnvironmentHeartbeatState(state networkpb.EnvironmentHeartbeatState) string {
	return strings.ToLower(strings.TrimPrefix(state.String(), "ENVIRONMENT_HEARTBEAT_STATE_"))
}
