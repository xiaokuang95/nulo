package sdk

import (
	"context"
	"strings"
	"testing"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
	"github.com/xiaokuang95/nulo/sdk/internal/session"
)

func TestNavigateTreeSummarize(t *testing.T) {
	p := ai.NewFauxProvider()
	p.SetResponses(ai.FauxAssistant("BRANCH SUM"))
	svcs, _ := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: t.TempDir(), AgentDir: t.TempDir(), InMemory: true, StreamFn: p.StreamSimple,
	})
	rt, _ := NewRuntime(RuntimeOptions{Services: svcs})
	defer rt.Close()
	sess, _ := rt.New(SessionStartOptions{Model: DefaultFauxModel(), Tools: []Tool{}})
	idUser, err := sess.Manager().AppendMessage(ai.UserMessage("q", 1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sess.Manager().AppendMessage(ai.AssistantMessage([]ai.ContentBlock{ai.TextBlock("abandoned")}, DefaultFauxModel(), ai.StopStop, 2)); err != nil {
		t.Fatal(err)
	}
	if err = sess.NavigateTree(context.Background(), idUser, NavigateTreeOptions{Summarize: true}); err != nil {
		t.Fatal(err)
	}
	llm := session.ConvertToLlm(sess.Manager().Project().Messages)
	found := false
	for _, m := range llm {
		if m.Role == "user" && strings.Contains(m.ContentString(), session.BranchSummaryPrefix) && strings.Contains(m.ContentString(), "BRANCH SUM") {
			found = true
		}
	}
	if !found {
		t.Fatalf("%+v", llm)
	}
}
