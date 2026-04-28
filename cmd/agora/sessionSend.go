package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/openziti/agora/sdk/agent"
	"github.com/openziti/agora/sdk/agent/session"
	"github.com/spf13/cobra"
)

func init() {
	sessionCmd.AddCommand(newSessionSendCommand().cmd)
}

type sessionSendCommand struct {
	workgroup     string
	messageType   string
	payload       string
	payloadFile   string
	contentType   string
	correlationID string
	timeout       time.Duration
	jsonOutput    bool
	cmd           *cobra.Command
}

func newSessionSendCommand() *sessionSendCommand {
	cmd := &cobra.Command{
		Use:   "send <adv_...>",
		Short: "One-shot: propose a session, send one envelope, receive the reply, close",
		Args:  cobra.ExactArgs(1),
	}
	command := &sessionSendCommand{cmd: cmd}
	cmd.Flags().StringVar(&command.workgroup, "workgroup", "", "Workgroup name or wg_... id (required)")
	cmd.Flags().StringVar(&command.messageType, "message-type", "", "Envelope message_type (required)")
	cmd.Flags().StringVar(&command.payload, "payload", "", "Inline string payload (UTF-8 bytes)")
	cmd.Flags().StringVar(&command.payloadFile, "payload-file", "", "File containing the payload bytes")
	cmd.Flags().StringVar(&command.contentType, "content-type", "", "Optional content_type hint for the payload")
	cmd.Flags().StringVar(&command.correlationID, "correlation-id", "", "Optional correlation_id")
	cmd.Flags().DurationVar(&command.timeout, "timeout", 30*time.Second, "Total round-trip timeout")
	cmd.Flags().BoolVarP(&command.jsonOutput, "json", "j", false, "Output raw JSON (header + base64 payload)")
	panicIfErr(cmd.MarkFlagRequired("workgroup"))
	panicIfErr(cmd.MarkFlagRequired("message-type"))
	cmd.Run = command.run
	return command
}

func (cmd *sessionSendCommand) run(_ *cobra.Command, args []string) {
	advID := args[0]

	app := agent.New("agora-session-send",
		agent.WithDescription("CLI one-shot envelope round-trip"),
		agent.WithRuntime(),
	)
	if err := app.Run(func(ctx context.Context, a *agent.Agent) error {
		wgID := resolveWorkgroupID(a.Controller(), cmd.workgroup)

		payload, err := cmd.payloadBytes()
		if err != nil {
			return err
		}

		sendCtx, cancel := context.WithTimeout(ctx, cmd.timeout)
		defer cancel()

		sess, err := session.Propose(sendCtx, a, advID, session.ProposeOptions{
			WorkgroupID: wgID,
			Timeout:     cmd.timeout / 2,
		})
		if err != nil {
			return fmt.Errorf("propose: %w", err)
		}

		out := session.Envelope{
			MessageType:   cmd.messageType,
			ContentType:   cmd.contentType,
			CorrelationID: cmd.correlationID,
			Payload:       payload,
		}
		if err := sess.Send(sendCtx, out); err != nil {
			_ = sess.Close(ctx, "send failed")
			return fmt.Errorf("send: %w", err)
		}
		reply, err := sess.Receive(sendCtx)
		if err != nil {
			_ = sess.Close(ctx, "receive failed")
			return fmt.Errorf("receive: %w", err)
		}
		if err := sess.Close(ctx, "one-shot complete"); err != nil {
			a.Log().Warnf("close after one-shot: %v", err)
		}

		if cmd.jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(map[string]any{
				"envelope_id":    reply.EnvelopeID,
				"message_type":   reply.MessageType,
				"content_type":   reply.ContentType,
				"correlation_id": reply.CorrelationID,
				"timestamp":      reply.Timestamp,
				"payload_b64":    base64.StdEncoding.EncodeToString(reply.Payload),
			})
		}
		fmt.Fprintf(os.Stderr, "reply: message_type=%s envelope_id=%s bytes=%d\n",
			reply.MessageType, reply.EnvelopeID, len(reply.Payload))
		_, _ = os.Stdout.Write(reply.Payload)
		fmt.Fprintln(os.Stdout)
		return nil
	}); err != nil {
		os.Exit(1)
	}
}

func (cmd *sessionSendCommand) payloadBytes() ([]byte, error) {
	if cmd.payload != "" && cmd.payloadFile != "" {
		return nil, fmt.Errorf("only one of --payload or --payload-file may be set")
	}
	if cmd.payloadFile != "" {
		return os.ReadFile(cmd.payloadFile)
	}
	return []byte(cmd.payload), nil
}
