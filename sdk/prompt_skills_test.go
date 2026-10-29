package sdk

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

func TestSystemPromptHasAgentsAndSkills(t *testing.T) {
	cwd := t.TempDir()
	os.WriteFile(filepath.Join(cwd, "AGENTS.md"), []byte("use nulo"), 0o644)
	sd := filepath.Join(cwd, ".nulo", "skills", "demo")
	os.MkdirAll(sd, 0o755)
	os.WriteFile(filepath.Join(sd, "SKILL.md"), []byte("---\nname: demo\ndescription: Demo skill\n---\n\nBODY\n"), 0o644)
	p := ai.NewFauxProvider()
	p.SetResponses(ai.FauxAssistant("ok"))
	svcs, _ := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: cwd, AgentDir: t.TempDir(), InMemory: true, ProjectTrusted: true, StreamFn: p.StreamSimple,
	})
	rt, _ := NewRuntime(RuntimeOptions{Services: svcs})
	defer rt.Close()
	sess, err := rt.New(SessionStartOptions{Model: DefaultFauxModel(), Tools: nil})
	if err != nil {
		t.Fatal(err)
	}
	proj := sess.Manager().Project()
	var sys *ai.Message
	for i := range proj.Messages {
		if proj.Messages[i].Role == "system" {
			sys = &proj.Messages[i]
		}
	}
	if sys == nil || sys.Sections["project_context"] == nil || !strings.Contains(*sys.Sections["project_context"], "use nulo") {
		t.Fatalf("missing AGENTS: %+v", sys)
	}
	if sys.Sections["skills"] == nil || !strings.Contains(*sys.Sections["skills"], "demo") {
		t.Fatalf("missing skills: %+v", sys)
	}
	p.SetResponses(ai.FauxAssistant("ok"))
	if err := sess.Prompt(context.Background(), "/skill:demo run"); err != nil {
		t.Fatal(err)
	}
}
