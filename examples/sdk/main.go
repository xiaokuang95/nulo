package main

import (
	"context"
	"fmt"
	"os"

	"github.com/xiaokuang95/nulo/sdk"
)

func main() {
	cwd, _ := os.Getwd()
	svcs, err := sdk.CreateAgentSessionServices(sdk.CreateAgentSessionServicesOptions{
		CWD:      cwd,
		AgentDir: sdk.DefaultAgentDir(),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	rt, err := sdk.NewRuntime(sdk.RuntimeOptions{Services: svcs})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer rt.Close()

	model, err := sdk.BuildModel(envOr("NULO_MODEL", "faux/faux-1"), svcs)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sess, err := rt.New(sdk.SessionStartOptions{Model: model})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	unsub := sess.Subscribe(func(ev sdk.Event) {
		fmt.Println("event:", ev.Type)
	})
	defer unsub()

	prompt := "Say hello"
	if len(os.Args) > 1 {
		prompt = os.Args[1]
	}
	if _, err := sess.Send(context.Background(), prompt); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if t := sess.LastAssistantText(); t != nil {
		fmt.Println("reply:", *t)
	}
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
