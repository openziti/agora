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
	advertiseCmd.AddCommand(newAdvertiseRetractCommand().cmd)
}

type advertiseRetractCommand struct {
	yes bool
	cmd *cobra.Command
}

func newAdvertiseRetractCommand() *advertiseRetractCommand {
	cmd := &cobra.Command{
		Use:   "retract <name|adv_...>",
		Short: "Retract an advertisement",
		Args:  cobra.ExactArgs(1),
	}
	command := &advertiseRetractCommand{cmd: cmd}
	cmd.Flags().BoolVarP(&command.yes, "yes", "y", false, "Skip interactive confirmation")
	cmd.Run = command.run
	return command
}

func (cmd *advertiseRetractCommand) run(_ *cobra.Command, args []string) {
	root := requireEnabledRoot()
	client := openEnvironmentAPIClient(root)
	advID := resolveAdvertisementID(client, args[0])

	if !cmd.yes {
		fmt.Printf("retract advertisement '%s'? Type 'yes' to confirm: ", advID)
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		if strings.TrimSpace(answer) != "yes" {
			fmt.Println("aborted")
			return
		}
	}

	res, err := client.RetractAdvertisement(context.Background(), api.RetractAdvertisementParams{AdvertisementId: advID})
	panicIfErr(err)
	switch typed := res.(type) {
	case *api.RetractAdvertisementNoContent:
		fmt.Printf("retracted advertisement '%s'\n", advID)
	case *api.RetractAdvertisementForbidden:
		panic(typed.Message)
	case *api.RetractAdvertisementNotFound:
		panic(typed.Message)
	case *api.RetractAdvertisementUnauthorized:
		panic(typed.Message)
	case *api.RetractAdvertisementInternalServerError:
		panic(typed.Message)
	default:
		panic(fmt.Sprintf("unexpected retract advertisement response: %T", res))
	}
}
