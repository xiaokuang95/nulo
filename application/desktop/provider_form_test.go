package main

import (
	"strings"
	"testing"
)

func TestProviderModelForms(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NULO_CODING_AGENT_SESSION_DIR", t.TempDir())
	a := newAppWithHost(hostOptions{Workspace: t.TempDir(), AgentDir: t.TempDir(), Trusted: true, Sync: true})
	defer a.closeHost()
	s := a.active()
	if s == nil {
		t.Fatal("no active session")
	}
	a.openProviderForm(s, "")
	a.providerFormID = "acme"
	a.providerFormName = "Acme"
	a.providerFormBaseURL = "https://acme.test/v1"
	a.providerFormAPI = "openai-completions"
	a.providerFormAPIKey = "$ACME_KEY"
	a.providerFormAdvanced = `{"headers":{"X-Test":"1"}}`
	a.finishProviderForm()
	if a.providerFormError != "" {
		t.Fatalf("provider form error = %s", a.providerFormError)
	}
	prov, ok := a.providerByID("acme")
	if !ok || prov.Name != "Acme" || prov.BaseURL != "https://acme.test/v1" {
		t.Fatalf("provider = %+v found=%v", prov, ok)
	}
	root, _, _, err := readJSONObject(a.modelsPath())
	if err != nil {
		t.Fatal(err)
	}
	providers := jsonObjectField(root, "providers")
	providerEntry, _ := providers["acme"].(map[string]any)
	headers, _ := providerEntry["headers"].(map[string]any)
	if headers["X-Test"] != "1" {
		t.Fatalf("provider advanced entry = %#v", providerEntry)
	}
	a.openModelForm(s, "acme", model{})
	a.modelFormID = "acme-1"
	a.modelFormName = "Acme One"
	a.modelFormContext = "64000"
	a.modelFormMaxOut = "4096"
	a.modelFormReasoning = true
	a.modelFormImage = true
	a.modelFormAdvanced = `{"cost":{"input":2}}`
	a.finishModelForm()
	if a.modelFormError != "" {
		t.Fatalf("model form error = %s", a.modelFormError)
	}
	prov, _ = a.providerByID("acme")
	if len(prov.Models) != 1 {
		t.Fatalf("models = %+v", prov.Models)
	}
	m := prov.Models[0]
	if m.ID != "acme-1" || m.Name != "Acme One" || m.Context != 64000 || m.MaxOut != 4096 || !m.Reasoning || !m.Image {
		t.Fatalf("model = %+v", m)
	}
	modelEntry := a.modelConfigEntry("acme", "acme-1")
	if cost, _ := modelEntry["cost"].(map[string]any); cost["input"] != float64(2) {
		t.Fatalf("model advanced entry = %#v", modelEntry)
	}
	a.openModelForm(s, "acme", m)
	if !strings.Contains(a.modelFormAdvanced, "cost") {
		t.Fatalf("model advanced not loaded: %q", a.modelFormAdvanced)
	}
	a.modelFormName = "Acme Updated"
	a.modelFormContext = "32000"
	a.modelFormImage = false
	a.finishModelForm()
	if a.modelFormError != "" {
		t.Fatalf("model edit error = %s", a.modelFormError)
	}
	prov, _ = a.providerByID("acme")
	m = prov.Models[0]
	if m.Name != "Acme Updated" || m.Context != 32000 || m.Image || !m.Reasoning {
		t.Fatalf("edited model = %+v", m)
	}
	a.openProviderForm(s, "acme")
	a.providerFormName = "Acme Labs"
	a.finishProviderForm()
	if a.providerFormError != "" {
		t.Fatalf("provider edit error = %s", a.providerFormError)
	}
	prov, _ = a.providerByID("acme")
	if prov.Name != "Acme Labs" || len(prov.Models) != 1 {
		t.Fatalf("edited provider = %+v", prov)
	}
	if err := a.deleteModel("acme", "acme-1"); err != nil {
		t.Fatal(err)
	}
	prov, _ = a.providerByID("acme")
	if len(prov.Models) != 0 {
		t.Fatalf("models after delete = %+v", prov.Models)
	}
	if err := a.deleteProvider("acme"); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.providerByID("acme"); ok {
		t.Fatal("provider still present after delete")
	}
}
