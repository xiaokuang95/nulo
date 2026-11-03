package sdk

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

func TestServicesUsesSettingsDefaults(t *testing.T) {
	agentDir, cwd := t.TempDir(), t.TempDir()
	writeModelCatalog(t, agentDir)
	if err := os.WriteFile(filepath.Join(agentDir, "settings.json"), []byte(`{
	  "defaultProvider": "p",
	  "defaultModel": "m",
	  "defaultThinkingLevel": "high",
	  "defaultTools": ["read", "edit"],
	  "cacheWarming": "idle"
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: cwd, AgentDir: agentDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if svcs.SettingsManager == nil {
		t.Fatal("services did not create a settings manager")
	}
	rt, err := NewRuntime(RuntimeOptions{Services: svcs})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	sess, err := rt.New(SessionStartOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := sess.Model(); got.Provider != "p" || got.ID != "m" {
		t.Fatalf("model %+v", got)
	}
	if got := sess.ThinkingLevel(); got != ThinkingHigh {
		t.Fatalf("thinking %q", got)
	}
	if sess.warmMode != "idle" {
		t.Fatalf("warm mode %q", sess.warmMode)
	}
	if got, want := toolNames(sess), []string{"read", "edit"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("tools %v, want %v", got, want)
	}
	if sess.SettingsManager() != svcs.SettingsManager {
		t.Fatal("session did not keep the services settings manager")
	}
}

func TestSettingsDefaultToolsModifiers(t *testing.T) {
	settings := NewInMemorySettingsManager(Settings{DefaultTools: []string{"+edit", "-bash"}})
	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: t.TempDir(), AgentDir: t.TempDir(), InMemory: true,
		SettingsManager: settings,
	})
	if err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(RuntimeOptions{Services: svcs})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	sess, err := rt.New(SessionStartOptions{Model: DefaultFauxModel()})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := toolNames(sess), []string{"read", "edit", "write"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("tools %v, want %v", got, want)
	}
}

func TestSettingsDiagnosticsAndSessionDir(t *testing.T) {
	agentDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(agentDir, "settings.json"), []byte(`{"broken":`), 0o644); err != nil {
		t.Fatal(err)
	}
	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: t.TempDir(), AgentDir: agentDir, InMemory: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(RuntimeOptions{Services: svcs})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	sess, err := rt.New(SessionStartOptions{Model: DefaultFauxModel()})
	if err != nil {
		t.Fatal(err)
	}
	diagnostics := sess.SettingsDiagnostics()
	if len(diagnostics) != 1 || diagnostics[0].Scope != "global" {
		t.Fatalf("diagnostics %+v", diagnostics)
	}
	if len(sess.SettingsDiagnostics()) != 0 {
		t.Fatal("diagnostics were not drained")
	}

	dir := t.TempDir()
	settings := NewInMemorySettingsManager(Settings{SessionDir: dir})
	svcs, err = CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: settings,
	})
	if err != nil {
		t.Fatal(err)
	}
	rt, err = NewRuntime(RuntimeOptions{Services: svcs})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	sess, err = rt.New(SessionStartOptions{Model: DefaultFauxModel(), InMemory: true})
	if err != nil {
		t.Fatal(err)
	}
	// InMemory overrides the session directory; exercise the persistent path.
	sess, err = rt.New(SessionStartOptions{Model: DefaultFauxModel()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sess.Manager().AppendMessage(ai.UserMessage("hi", 1)); err != nil {
		t.Fatal(err)
	}
	if got := filepath.Dir(sess.Manager().File()); got != dir {
		t.Fatalf("session dir %q, want %q", got, dir)
	}
	list, err := rt.ListSessions()
	if err != nil || len(list) != 1 {
		t.Fatalf("list %+v err %v", list, err)
	}
}

func TestSettingsDefaultToolsCanSelectCodemode(t *testing.T) {
	settings := NewInMemorySettingsManager(Settings{DefaultTools: []string{"read", "codemode"}})
	svcs, err := CreateAgentSessionServices(CreateAgentSessionServicesOptions{
		CWD: t.TempDir(), AgentDir: t.TempDir(), InMemory: true,
		SettingsManager: settings,
	})
	if err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(RuntimeOptions{Services: svcs})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	sess, err := rt.New(SessionStartOptions{Model: DefaultFauxModel(), InMemory: true})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := toolNames(sess), []string{"read", "codemode"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("tools %v, want %v", got, want)
	}
}
