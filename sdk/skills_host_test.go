package sdk

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDisabledSkillsFilterSession(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	agentDir := t.TempDir()
	cwd := t.TempDir()
	for _, name := range []string{"one", "two"} {
		dir := filepath.Join(agentDir, "skills", name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		body := "---\nname: " + name + "\ndescription: " + name + " skill\n---\n\nBody.\n"
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: cwd, AgentDir: agentDir, InMemory: true, DisabledSkills: []string{"one"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer svcs.Close()
	rt, err := NewRuntime(RuntimeOptions{Services: svcs})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	sess, err := rt.New(SessionStartOptions{Model: DefaultFauxModel()})
	if err != nil {
		t.Fatal(err)
	}
	names := sess.SkillNames()
	if !containsString(names, "two") || containsString(names, "one") {
		t.Fatalf("session skills = %v, want two without one", names)
	}
	var foundOne, foundTwo bool
	for _, skill := range ListSkills(agentDir, cwd, true) {
		foundOne = foundOne || skill.Name == "one"
		foundTwo = foundTwo || skill.Name == "two"
	}
	if !foundOne || !foundTwo {
		t.Fatalf("ListSkills missed a configured skill: one=%v two=%v", foundOne, foundTwo)
	}
}
