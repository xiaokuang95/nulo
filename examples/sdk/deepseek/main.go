// Command deepseek runs the nulo SDK against a real chat-completions model.
//
//	go run ./examples/sdk/deepseek "用一句话解释什么是 goroutine"
//
// Unlike examples/sdk/main.go, which runs offline against the faux provider, this
// example talks to a real endpoint and therefore needs two things in the nulo
// config root (~/.nulo/agent, overridable with NULO_CODING_AGENT_DIR):
//
//	models.json  a model whose api is "openai-completions"
//	auth.json    an api_key entry for that model's provider
//
// The model spec defaults to tokenharbor/deepseek-v4.1-flash and can be changed
// with NULO_MODEL.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/xiaokuang95/nulo/sdk"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	agentDir := sdk.DefaultAgentDir()
	spec := envOr("NULO_MODEL", "tokenharbor/deepseek-v4.1-flash")
	prompt := "用一句话解释什么是 goroutine。"
	if len(os.Args) > 1 {
		prompt = strings.Join(os.Args[1:], " ")
	}

	svcs, err := sdk.CreateAgentSessionServices(sdk.CreateAgentSessionServicesOptions{
		CWD:      cwd,
		AgentDir: agentDir,
	})
	if err != nil {
		return err
	}
	rt, err := sdk.NewRuntime(sdk.RuntimeOptions{Services: svcs})
	if err != nil {
		return err
	}
	defer rt.Close()

	model, err := sdk.BuildModel(spec, svcs)
	if err != nil {
		return fmt.Errorf("%w\n\nadd it to %s/models.json", err, agentDir)
	}
	fmt.Printf("model  : %s/%s (%s)\n", model.Provider, model.ID, model.API)
	fmt.Printf("base   : %s\n", model.BaseURL)
	fmt.Printf("agent  : %s\n", agentDir)
	fmt.Printf("prompt : %s\n\n", prompt)

	// A fresh in-memory session keeps a smoke test from writing JSONL into the
	// real session directory; drop InMemory to persist the run.
	sess, err := rt.New(sdk.SessionStartOptions{Model: model, InMemory: true})
	if err != nil {
		return err
	}
	defer sess.Dispose()

	unsub := sess.Subscribe(onEvent)
	defer unsub()

	msgs, err := sess.Send(context.Background(), prompt)
	if err != nil {
		// A nil error does not mean the model answered: a failed request still
		// returns successfully and reports the failure on the assistant message.
		return err
	}
	fmt.Println()

	for _, m := range msgs {
		if m.Role == "assistant" {
			reportAssistant(m)
		}
	}
	return nil
}

// onEvent prints the loop's progress. The SDK delivers events synchronously, so
// this callback must stay short.
func onEvent(ev sdk.Event) {
	switch ev.Type {
	case "agent_start":
		fmt.Println("▶ agent_start")
	case "turn_start":
		fmt.Println("▶ turn_start")
	case "message_start":
		if ev.Message != nil {
			fmt.Printf("  · %s\n", ev.Message.Role)
		}
	case "message_update":
		if ev.AssistantMessageEvent != nil && ev.AssistantMessageEvent.Type == "text_delta" {
			fmt.Print(ev.AssistantMessageEvent.Delta)
		}
	case "tool_execution_start":
		fmt.Printf("  ⚙ %s %v\n", ev.ToolName, ev.Args)
	case "tool_execution_end":
		fmt.Printf("  ⚙ %s done isError=%v\n", ev.ToolName, ev.IsError)
	case "turn_end":
		fmt.Println("■ turn_end")
	case "agent_settled":
		fmt.Println("✓ agent_settled")
	}
}

// reportAssistant prints what the model actually produced. Callers must check
// StopReason rather than the error return: an unavailable key, a bad endpoint or
// an HTTP failure all arrive here as StopError plus ErrorMessage.
func reportAssistant(m sdk.Message) {
	fmt.Printf("stopReason: %s\n", m.StopReason)
	if m.ErrorMessage != "" {
		fmt.Printf("error     : %s\n", m.ErrorMessage)
	}
	if m.Usage != nil {
		u := m.Usage
		fmt.Printf("usage     : in=%d out=%d cacheRead=%d cacheWrite=%d", u.Input, u.Output, u.CacheRead, u.CacheWrite)
		if u.Reasoning != nil {
			fmt.Printf(" reasoning=%d", *u.Reasoning)
		}
		fmt.Printf(" total=%d cost=%.6f\n", u.TotalTokens, u.Cost.Total)
	}
	if text := messageText(m); text != "" {
		fmt.Printf("text      : %s\n", text)
	}
}

func messageText(m sdk.Message) string {
	var parts []string
	for _, b := range m.ContentBlocks() {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
