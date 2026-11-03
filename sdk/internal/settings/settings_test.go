package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

func ptr[T any](v T) *T { return &v }

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDefaults(t *testing.T) {
	m := New(t.TempDir(), t.TempDir())
	if m.GetDefaultProvider() != "" || m.GetDefaultModel() != "" || m.GetDefaultThinkingLevel() != "" {
		t.Fatal("defaults are not empty")
	}
	if m.GetDefaultTools() != nil {
		t.Fatal("default tools should be unset")
	}
	if got, want := m.ResolvedDefaultTools(), []string{"read", "bash", "edit", "write"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("resolved tools %v", got)
	}
	config, err := m.GetCompactionSettings()
	if err != nil || config != (CompactionConfig{Enabled: true, ReserveTokens: 16384, KeepRecentTokens: 20000}) {
		t.Fatalf("compaction %+v %v", config, err)
	}
	if branch := m.GetBranchSummarySettings(); branch.ReserveTokens != 16384 || branch.SkipPrompt {
		t.Fatalf("branch %+v", branch)
	}
	retry := m.GetRetrySettings()
	if !retry.Enabled || retry.MaxRetries != 3 || retry.BaseDelayMs != 2000 || retry.MaxAgentDelayMs != 60000 {
		t.Fatalf("retry %+v", retry)
	}
	if provider := m.GetProviderRetrySettings(); provider.MaxRetryDelayMs != 60000 {
		t.Fatalf("provider retry %+v", provider)
	}
	if m.GetCacheWarmingMode() != "streaming" || m.GetTransport() != "auto" || m.GetSteeringMode() != "one-at-a-time" {
		t.Fatal("string defaults")
	}
	if !m.GetShowImages() || m.GetImageWidthCells() != 60 || !m.GetImageAutoResize() || m.GetBlockImages() {
		t.Fatal("image defaults")
	}
	if !m.GetEnableSkillCommands() || m.GetDoubleEscapeAction() != "tree" || m.GetTreeFilterMode() != "default" {
		t.Fatal("UI defaults")
	}
	if m.GetCodeBlockIndent() != "  " || m.GetCodemodeInlineBudget() != 3000 || m.GetCodemodeMode() != "on" {
		t.Fatal("markdown/codemode defaults")
	}
	if got := m.GetWarnings(); got.AnthropicExtraUsage == nil || !*got.AnthropicExtraUsage {
		t.Fatalf("warnings %+v", got)
	}
	if got := m.GetSkillPaths(); !reflect.DeepEqual(got, []string{}) {
		t.Fatalf("skills %v", got)
	}
}

func TestGlobalProjectMergeAndTrust(t *testing.T) {
	agentDir, cwd := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(agentDir, "settings.json"), `{
	  "defaultProvider": "global",
	  "modelThinkingLevels": {"a/b": "low"},
	  "compaction": {"enabled": false, "reserveTokens": 1111},
	  "defaultTools": ["read", "bash"],
	  "skills": ["global-skill"],
	  "retry": {"provider": {"maxRetryDelayMs": 7}},
	  "unknown": {"keep": 1}
	}`)
	writeFile(t, filepath.Join(cwd, ".nulo", "settings.json"), `{
	  "defaultModel": "project",
	  "modelThinkingLevels": {"c/d": "high"},
	  "compaction": {"keepRecentTokens": 2222},
	  "defaultTools": ["+edit", "-bash"],
	  "skills": ["project-skill"],
	  "unknown": {"extra": "x"},
	  "newKey": true
	}`)
	trusted := true
	m := NewWithOptions(cwd, agentDir, Options{ProjectTrusted: &trusted})
	if m.GetDefaultProvider() != "global" || m.GetDefaultModel() != "project" {
		t.Fatalf("models %q %q", m.GetDefaultProvider(), m.GetDefaultModel())
	}
	config, err := m.GetCompactionSettings()
	if err != nil || config.Enabled || config.ReserveTokens != 1111 || config.KeepRecentTokens != 2222 {
		t.Fatalf("compaction %+v %v", config, err)
	}
	if got, want := m.GetDefaultTools(), []string{"read", "edit"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("tools %v", got)
	}
	if got := m.GetModelThinkingLevel("a", "b"); got != ai.ThinkingLow {
		t.Fatalf("model level %q", got)
	}
	if got := m.GetModelThinkingLevel("c", "d"); got != ai.ThinkingHigh {
		t.Fatalf("project model level %q", got)
	}
	if got := m.GetSkillPaths(); !reflect.DeepEqual(got, []string{"project-skill"}) {
		t.Fatalf("skills %v", got)
	}
	if retry := m.GetProviderRetrySettings(); retry.MaxRetryDelayMs != 7 {
		t.Fatalf("retry %+v", retry)
	}
	merged := m.Get()
	var unknown map[string]any
	if err := json.Unmarshal(merged.Extra["unknown"], &unknown); err != nil || unknown["keep"] != float64(1) || unknown["extra"] != "x" {
		t.Fatalf("unknown merge %s (%v)", merged.Extra["unknown"], err)
	}
	if _, ok := merged.Extra["newKey"]; !ok {
		t.Fatal("new project key missing")
	}

	untrusted := false
	u := NewWithOptions(cwd, agentDir, Options{ProjectTrusted: &untrusted})
	if u.GetDefaultModel() != "" {
		t.Fatal("untrusted project settings were read")
	}
	if got, want := u.GetDefaultTools(), []string{"read", "bash"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("untrusted tools %v", got)
	}
	u.SetProjectTrusted(true)
	if u.GetDefaultModel() != "project" {
		t.Fatal("trusting the project did not load it")
	}
	u.SetProjectTrusted(false)
	if u.GetDefaultModel() != "" {
		t.Fatal("untrusting the project kept its values")
	}
}

func TestSettersPersistAndPreserveUnknown(t *testing.T) {
	agentDir, cwd := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(agentDir, "settings.json"), `{"unknown":{"a":1},"defaultProvider":"old"}`)
	m := NewWithOptions(cwd, agentDir, Options{ProjectTrusted: ptr(true)})
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(m.SetDefaultProvider("prov"))
	must(m.SetDefaultModel("model"))
	must(m.SetDefaultThinkingLevel(ai.ThinkingHigh))
	must(m.SetModelThinkingLevel("p", "m", ai.ThinkingLow))
	must(m.SetCompactionEnabled(false))
	must(m.SetCompactionReserveTokens(1))
	must(m.SetCompactionKeepRecentTokens(2))
	must(m.SetBranchSummaryReserveTokens(3))
	must(m.SetBranchSummarySkipPrompt(true))
	must(m.SetCacheWarmingMode("off"))
	must(m.SetDefaultTools([]string{}))
	must(m.SetEnabledModels([]string{"a/*"}))
	must(m.SetSkillPaths([]string{"skill"}))
	must(m.SetPromptTemplatePaths([]string{"prompt"}))
	must(m.SetThemePaths([]string{"theme"}))
	must(m.SetSessionDir("~/sessions"))
	must(m.SetShellPath("~/bin/bash"))
	must(m.SetShellCommandPrefix("set -e\n"))
	must(m.SetHTTPProxy("http://proxy"))
	must(m.SetCodemodeInlineBudget(42))
	must(m.SetWarnings(WarningsSettings{AnthropicExtraUsage: ptr(false)}))
	must(m.SetProjectSkillPaths([]string{"project-skill"}))

	reloaded := NewWithOptions(cwd, agentDir, Options{ProjectTrusted: ptr(true)})
	if reloaded.GetDefaultProvider() != "prov" || reloaded.GetDefaultModel() != "model" || reloaded.GetDefaultThinkingLevel() != ai.ThinkingHigh {
		t.Fatal("defaults did not persist")
	}
	if got := reloaded.GetModelThinkingLevel("p", "m"); got != ai.ThinkingLow {
		t.Fatalf("model level %q", got)
	}
	if config, err := reloaded.GetCompactionSettings(); err != nil || config.Enabled || config.ReserveTokens != 1 || config.KeepRecentTokens != 2 {
		t.Fatalf("compaction %+v %v", config, err)
	}
	if branch := reloaded.GetBranchSummarySettings(); branch.ReserveTokens != 3 || !branch.SkipPrompt {
		t.Fatalf("branch %+v", branch)
	}
	if reloaded.GetCacheWarmingMode() != "off" {
		t.Fatal("cache warming")
	}
	if tools := reloaded.GetDefaultTools(); tools == nil || len(tools) != 0 {
		t.Fatalf("default tools %v", tools)
	}
	if got := reloaded.GetEnabledModels(); !reflect.DeepEqual(got, []string{"a/*"}) {
		t.Fatalf("enabled %v", got)
	}
	if got := reloaded.GetSkillPaths(); !reflect.DeepEqual(got, []string{"project-skill"}) {
		t.Fatalf("skills %v", got)
	}
	if got, _ := reloaded.GetSessionDir(); filepath.Base(got) != "sessions" {
		t.Fatalf("session dir %q", got)
	}
	if reloaded.GetShellCommandPrefix() != "set -e\n" || reloaded.GetHTTPProxy() != "http://proxy" {
		t.Fatal("shell/proxy")
	}
	if reloaded.GetCodemodeInlineBudget() != 42 {
		t.Fatal("codemode budget")
	}
	if got := reloaded.GetWarnings(); got.AnthropicExtraUsage == nil || *got.AnthropicExtraUsage {
		t.Fatalf("warnings %+v", got)
	}
	global := readFileMap(t, filepath.Join(agentDir, "settings.json"))
	if _, ok := global["unknown"]; !ok {
		t.Fatal("unknown key was dropped")
	}
	if tools, ok := global["defaultTools"].([]any); !ok || len(tools) != 0 {
		t.Fatalf("empty defaultTools not written: %#v", global["defaultTools"])
	}
}

func readFileMap(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	return object
}

func TestParseErrorsAndBOM(t *testing.T) {
	agentDir := t.TempDir()
	writeFile(t, filepath.Join(agentDir, "settings.json"), "\ufeff{\"defaultProvider\":\"bom\"}")
	m := New(t.TempDir(), agentDir)
	if got := m.GetDefaultProvider(); got != "bom" {
		t.Fatalf("BOM provider %q", got)
	}
	if errors := m.DrainErrors(); len(errors) != 0 {
		t.Fatalf("BOM errors %+v", errors)
	}

	badDir := t.TempDir()
	writeFile(t, filepath.Join(badDir, "settings.json"), `{"broken":`)
	m = New(t.TempDir(), badDir)
	if errors := m.DrainErrors(); len(errors) != 1 || errors[0].Scope != "global" {
		t.Fatalf("errors %+v", errors)
	}
	if err := m.SetDefaultProvider("x"); err == nil {
		t.Fatal("a setter must refuse to overwrite a malformed file")
	}
}

func TestInMemoryIsolation(t *testing.T) {
	agentDir := t.TempDir()
	m := NewInMemory(Settings{DefaultProvider: "memory", Compaction: &CompactionSettings{ReserveTokens: ptr(100)}})
	if m.GetDefaultProvider() != "memory" {
		t.Fatal("initial setting")
	}
	if _, err := m.GetSessionDir(); err != nil {
		t.Fatal(err)
	}
	if err := m.SetDefaultModel("changed"); err != nil {
		t.Fatal(err)
	}
	m.Reload()
	if m.GetDefaultModel() != "changed" {
		t.Fatal("reload lost an in-memory setter")
	}
	if _, err := os.Stat(filepath.Join(agentDir, "settings.json")); !os.IsNotExist(err) {
		t.Fatalf("in-memory manager wrote a file: %v", err)
	}
}

func TestCompactionModelOverridesAndInvalidNumbers(t *testing.T) {
	model := ai.Model{Provider: "p", ID: "m"}
	m := NewInMemory(Settings{Compaction: &CompactionSettings{
		ReserveTokens: ptr(8192),
		ModelOverrides: map[string]CompactionModelOverride{
			"p/m": {ReserveTokens: ptr(400000)},
		},
	}})
	if value, err := m.GetCompactionReserveTokens(model); err != nil || value != 400000 {
		t.Fatalf("override %d %v", value, err)
	}
	if value, err := m.GetCompactionReserveTokens(); err != nil || value != 8192 {
		t.Fatalf("ordinary %d %v", value, err)
	}

	badDir := t.TempDir()
	writeFile(t, filepath.Join(badDir, "settings.json"), `{"compaction":{"reserveTokens":1.5}}`)
	bad := New(t.TempDir(), badDir)
	if _, err := bad.GetCompactionSettings(); err == nil {
		t.Fatal("fractional reserveTokens must fail")
	}
}

func TestToolModifiers(t *testing.T) {
	if got := ApplyToolModifiers([]string{"read", "bash"}, []string{"+edit", "-bash", "+bash", "-missing"}); !reflect.DeepEqual(got, []string{"read", "edit", "bash"}) {
		t.Fatalf("modifiers %v", got)
	}
	if got := MergeDefaultTools([]string{"read"}, []string{"+edit"}); !reflect.DeepEqual(got, []string{"read", "+edit"}) {
		t.Fatalf("merge %v", got)
	}
	if got := MergeDefaultTools([]string{"read"}, []string{"bash"}); !reflect.DeepEqual(got, []string{"bash"}) {
		t.Fatalf("replace %v", got)
	}
}

func TestMigrations(t *testing.T) {
	agentDir := t.TempDir()
	writeFile(t, filepath.Join(agentDir, "settings.json"), `{
	  "queueMode": "all",
	  "websockets": true,
	  "skills": {"customDirectories": ["one"], "enableSkillCommands": false},
	  "retry": {"maxDelayMs": 9}
	}`)
	m := New(t.TempDir(), agentDir)
	if m.GetSteeringMode() != "all" || m.GetTransport() != "websocket" {
		t.Fatalf("queue/transport %q %q", m.GetSteeringMode(), m.GetTransport())
	}
	if got := m.GetSkillPaths(); !reflect.DeepEqual(got, []string{"one"}) || m.GetEnableSkillCommands() {
		t.Fatalf("skills %v enabled %v", got, m.GetEnableSkillCommands())
	}
	if retry := m.GetProviderRetrySettings(); retry.MaxRetryDelayMs != 9 {
		t.Fatalf("retry %+v", retry)
	}
}

func TestSettingsWritePreservesKeyOrder(t *testing.T) {
	agentDir := t.TempDir()
	path := filepath.Join(agentDir, "settings.json")
	writeFile(t, path, `{"zeta":1,"defaultProvider":"x","alpha":{"z":1,"a":2},"mid":true}`)
	m := New(t.TempDir(), agentDir)
	if err := m.SetTheme("dark"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"zeta\": 1,\n  \"defaultProvider\": \"x\",\n  \"alpha\": {\n    \"z\": 1,\n    \"a\": 2\n  },\n  \"mid\": true,\n  \"theme\": \"dark\"\n}"
	if string(got) != want {
		t.Fatalf("settings = %q\nwant %q", got, want)
	}

	writeFile(t, path, `{"compaction":{"enabled":true,"keepRecentTokens":1,"reserveTokens":2},"theme":"x"}`)
	m = New(t.TempDir(), agentDir)
	if err := m.SetCompactionKeepRecentTokens(9); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(path)
	want = "{\n  \"compaction\": {\n    \"enabled\": true,\n    \"keepRecentTokens\": 9,\n    \"reserveTokens\": 2\n  },\n  \"theme\": \"x\"\n}"
	if string(got) != want {
		t.Fatalf("nested settings = %q\nwant %q", got, want)
	}
}

func TestInMemoryDefaultToolsSemantics(t *testing.T) {
	if got := NewInMemory(Settings{}).GetDefaultTools(); got != nil {
		t.Fatalf("absent defaultTools = %v", got)
	}
	if got := NewInMemory(Settings{DefaultTools: []string{}}).GetDefaultTools(); got == nil || len(got) != 0 {
		t.Fatalf("empty defaultTools = %v", got)
	}
	if got := NewInMemory(Settings{DefaultTools: []string{"+codemode", "-write"}}).GetDefaultTools(); !reflect.DeepEqual(got, []string{"read", "bash", "edit", "codemode"}) {
		t.Fatalf("modifiers = %v", got)
	}
	if got := NewInMemory(Settings{DefaultTools: []string{"read", "+grep", "+read"}}).GetDefaultTools(); !reflect.DeepEqual(got, []string{"read", "grep"}) {
		t.Fatalf("dedupe = %v", got)
	}
}

func TestMergeDefaultToolsUpstreamTable(t *testing.T) {
	for _, tc := range []struct {
		name            string
		base, overrides []string
		want            []string
	}{
		{"no override keeps the base", []string{"read"}, nil, []string{"read"}},
		{"no base takes the override", nil, []string{"+a"}, []string{"+a"}},
		{"plain names replace", []string{"read", "+a"}, []string{"grep"}, []string{"grep"}},
		{"modifiers append", []string{"read"}, []string{"+a", "-read"}, []string{"read", "+a", "-read"}},
		{"mixed list replaces", []string{"read"}, []string{"+a", "grep"}, []string{"+a", "grep"}},
		{"empty override keeps an inherited list", []string{"read"}, []string{}, []string{"read"}},
		{"empty override without a base stays empty", nil, []string{}, []string{}},
		{"empty override on an empty base stays empty", []string{}, []string{}, []string{}},
	} {
		if got := MergeDefaultTools(tc.base, tc.overrides); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: merged = %#v, want %#v", tc.name, got, tc.want)
		}
	}
}

func TestSettingsWriteMatchesJSStringifyOrdering(t *testing.T) {
	agentDir := t.TempDir()
	path := filepath.Join(agentDir, "settings.json")
	writeFile(t, path, `{"10":1,"2":2,"a":3,"1":4,"-1":5,"n":1.0,"e":1e-7,"b":"a<b>&c","u":"\u2028"}`)
	m := New(t.TempDir(), agentDir)
	if err := m.SetTheme("dark"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	want := "{\n  \"1\": 4,\n  \"2\": 2,\n  \"10\": 1,\n  \"a\": 3,\n  \"-1\": 5,\n  \"n\": 1,\n  \"e\": 1e-7,\n  \"b\": \"a<b>&c\",\n  \"u\": \"\u2028\",\n  \"theme\": \"dark\"\n}"
	if string(got) != want {
		t.Fatalf("settings = %q\nwant %q", got, want)
	}
}

func TestSettingsWriteAppliesMigrations(t *testing.T) {
	agentDir := t.TempDir()
	path := filepath.Join(agentDir, "settings.json")
	writeFile(t, path, `{"theme":"dark","queueMode":"all","websockets":true,"skills":{"customDirectories":["one"]},"retry":{"enabled":true,"maxDelayMs":9}}`)
	m := New(t.TempDir(), agentDir)
	if err := m.SetDefaultModel("next"); err != nil {
		t.Fatal(err)
	}
	global := readFileMap(t, path)
	if _, ok := global["queueMode"]; ok {
		t.Fatal("queueMode was not migrated")
	}
	if global["steeringMode"] != "all" || global["transport"] != "websocket" {
		t.Fatalf("migration values %#v %#v", global["steeringMode"], global["transport"])
	}
	if _, ok := global["websockets"]; ok {
		t.Fatal("websockets was not migrated")
	}
	if tools, ok := global["skills"].([]any); !ok || len(tools) != 1 || tools[0] != "one" {
		t.Fatalf("skills %#v", global["skills"])
	}
	retry, _ := global["retry"].(map[string]any)
	provider, _ := retry["provider"].(map[string]any)
	if provider["maxRetryDelayMs"] != float64(9) {
		t.Fatalf("retry %#v", global["retry"])
	}
	if _, ok := retry["maxDelayMs"]; ok {
		t.Fatal("maxDelayMs was not migrated")
	}
}
