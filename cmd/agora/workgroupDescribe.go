package main

import (
	"context"
	"fmt"

	"github.com/openziti/agora/internal/api"
	"github.com/openziti/agora/internal/clioutput"
	"github.com/spf13/cobra"
)

func init() {
	workgroupCmd.AddCommand(newWorkgroupDescribeCommand().cmd)
}

type workgroupDescribeCommand struct {
	jsonOutput bool
	cmd        *cobra.Command
}

func newWorkgroupDescribeCommand() *workgroupDescribeCommand {
	cmd := &cobra.Command{
		Use:   "describe <name|wg_...>",
		Short: "Describe a workgroup",
		Args:  cobra.ExactArgs(1),
	}
	command := &workgroupDescribeCommand{cmd: cmd}
	cmd.Flags().BoolVarP(&command.jsonOutput, "json", "j", false, "Output raw JSON instead of a summary")
	cmd.Run = command.run
	return command
}

func (cmd *workgroupDescribeCommand) run(_ *cobra.Command, args []string) {
	root := requireEnabledRoot()
	client := openEnvironmentAPIClient(root)
	wgID := resolveWorkgroupID(client, args[0])

	res, err := client.GetWorkgroup(context.Background(), api.GetWorkgroupParams{WorkgroupId: wgID})
	panicIfErr(err)

	wg, ok := res.(*api.Workgroup)
	if !ok {
		switch typed := res.(type) {
		case *api.GetWorkgroupNotFound:
			panic(typed.Message)
		case *api.GetWorkgroupUnauthorized:
			panic(typed.Message)
		case *api.GetWorkgroupInternalServerError:
			panic(typed.Message)
		}
		panic(fmt.Sprintf("unexpected get workgroup response: %T", res))
	}

	if cmd.jsonOutput {
		panicIfErr(clioutput.RenderJSON(wg))
		return
	}

	fmt.Printf("Name:                %s\n", wg.Name)
	fmt.Printf("ID:                  %s\n", wg.ID)
	fmt.Printf("Scope:               %s\n", wg.Scope)
	fmt.Printf("State:               %s\n", wg.State)
	fmt.Printf("Owner organization:  %s\n", wg.OwnerOrganizationId)
	if wg.Description.Set {
		fmt.Printf("Description:         %s\n", wg.Description.Value)
	}
	if len(wg.ParticipatingOrganizationIds) > 0 {
		fmt.Printf("Participating orgs:  %v\n", wg.ParticipatingOrganizationIds)
	}
	fmt.Printf("Created at:          %s\n", clioutput.TimeUTC(wg.CreatedAt))
	fmt.Printf("Updated at:          %s\n", clioutput.TimeUTC(wg.UpdatedAt))
}
