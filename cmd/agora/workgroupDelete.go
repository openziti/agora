package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/openziti/agora/internal/api"
	"github.com/spf13/cobra"
)

func init() {
	workgroupCmd.AddCommand(newWorkgroupDeleteCommand().cmd)
}

type workgroupDeleteCommand struct {
	yes bool
	cmd *cobra.Command
}

func newWorkgroupDeleteCommand() *workgroupDeleteCommand {
	cmd := &cobra.Command{
		Use:   "delete <name|wg_...>",
		Short: "Delete a workgroup",
		Args:  cobra.ExactArgs(1),
	}
	command := &workgroupDeleteCommand{cmd: cmd}
	cmd.Flags().BoolVarP(&command.yes, "yes", "y", false, "Skip interactive confirmation")
	cmd.Run = command.run
	return command
}

func (cmd *workgroupDeleteCommand) run(_ *cobra.Command, args []string) {
	root := requireEnabledRoot()
	client := openEnvironmentAPIClient(root)
	wgID := resolveWorkgroupID(client, args[0])

	if !cmd.yes {
		fmt.Printf("delete workgroup '%s'? Type 'yes' to confirm: ", wgID)
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		if strings.TrimSpace(answer) != "yes" {
			fmt.Println("aborted")
			return
		}
	}

	res, err := client.DeleteWorkgroup(context.Background(), api.DeleteWorkgroupParams{WorkgroupId: wgID})
	panicIfErr(err)
	switch typed := res.(type) {
	case *api.DeleteWorkgroupNoContent:
		fmt.Printf("deleted workgroup '%s'\n", wgID)
	case *api.DeleteWorkgroupForbidden:
		panic(typed.Message)
	case *api.DeleteWorkgroupNotFound:
		panic(typed.Message)
	case *api.DeleteWorkgroupUnauthorized:
		panic(typed.Message)
	case *api.DeleteWorkgroupInternalServerError:
		panic(typed.Message)
	default:
		panic(fmt.Sprintf("unexpected delete workgroup response: %T", res))
	}
}
