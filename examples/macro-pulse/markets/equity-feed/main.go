package main

import (
	"context"
	"flag"
	"os"

	"github.com/openziti/agora/sdk/agent"
)

func main() {
	fs := flag.NewFlagSet("equity-feed", flag.ExitOnError)

	app := agent.New("equity-feed",
		agent.WithDescription("Major US equity indices and sector ETFs"),
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
