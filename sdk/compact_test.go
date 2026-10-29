package sdk

import (
	"context"
	"strings"
	"testing"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
	"github.com/xiaokuang95/nulo/sdk/internal/session"
)

func TestCompactManual(t *testing.T) {
	p := ai.NewFauxProvider()
	svcs, _ := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: t.TempDir(), AgentDir: t.TempDir(), InMemory: true, StreamFn: p.StreamSimple,
	})
	rt, _ := NewRuntime(RuntimeOptions{Services: svcs})
	defer rt.Close()
	sess, _ := rt.New(SessionStartOptions{Model: DefaultFauxModel(), Tools: []Tool{}})
	_, err := sess.Compact(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "Nothing to compact") {
		t.Fatalf("small: %v", err)
	}
	blob := strings.Repeat("word ", 5000)
	for i := 0; i < 20; i++ {
		sess.Manager().AppendMessage(ai.UserMessage(blob, int64(i+1)))
	}
	p.SetResponses(ai.FauxAssistant("SUMMARIZED"))
	res, err := sess.Compact(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary != "SUMMARIZED" {
		t.Fatalf("%+v", res)
	}
	proj := sess.Manager().Project()
	llm := session.ConvertToLlm(proj.Messages)
	found := false
	for _, m := range llm {
		if m.Role == "user" && strings.Contains(m.ContentString(), session.CompactionSummaryPrefix) && strings.Contains(m.ContentString(), "SUMMARIZED") {
			found = true
		}
	}
	if !found {
		t.Fatalf("projection missing summary user: %+v", llm)
	}
}
