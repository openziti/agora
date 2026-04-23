package main

import (
	"context"
	"flag"
	"os"

	"github.com/openziti/agora/sdk/agent"
)

func main() {
	fs := flag.NewFlagSet("correlator", flag.ExitOnError)

	app := agent.New("correlator",
		agent.WithDescription("Pearson correlation coefficient between two aligned time series"),
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
