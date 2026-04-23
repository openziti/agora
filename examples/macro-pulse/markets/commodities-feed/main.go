package main

import (
	"context"
	"flag"
	"os"

	"github.com/openziti/agora/sdk/agent"
)

func main() {
	fs := flag.NewFlagSet("commodities-feed", flag.ExitOnError)

	app := agent.New("commodities-feed",
		agent.WithDescription("Front-month commodity spot prices and recent history"),
		agent.WithFlagSet(fs),
		agent.WithRuntime(),
	)
	if err := app.Run(func(ctx context.Context, a *agent.Agent) error {
		a.Log().Info("alive")
		<-ctx.Done()
		return nil
	}); err != nil {
		os.Exit(1)
	}
}
