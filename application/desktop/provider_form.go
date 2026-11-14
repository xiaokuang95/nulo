package main

import (
	"errors"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

var providerFormHandled = jsonSet("name", "baseUrl", "api", "apiKey", "models")

var modelFormHandled = jsonSet("id", "name", "contextWindow", "maxTokens", "reasoning", "input")

func (a *app) modelsPath() string {
	if a == nil || a.host == nil {
		return ""
	}
	return filepath.Join(a.host.agentDir, "models.json")
}

func (a *app) providerFormSession() *session {
	for _, s := range a.sessions {
		if s.ID == a.providerFormSessionID {
			return s
		}
	}
	return a.active()
}

func (a *app) modelFormSession() *session {
	for _, s := range a.sessions {
		if s.ID == a.modelFormSessionID {
			return s
		}
	}
	return a.active()
}

func validProviderID(id string) bool {
	if id == "" || id != strings.TrimSpace(id) {
		return false
	}
	return !strings.ContainsAny(id, " \t\r\n/\\")
}

func validModelID(id string) bool {
	if id == "" || id != strings.TrimSpace(id) {
		return false
	}
	return !strings.ContainsAny(id, " \t\r\n")
}

func (a *app) openProviderForm(s *session, id string) {
	if a == nil || a.host == nil {
		return
	}
	a.providerFormAdd = id == ""
	a.providerFormID = id
	a.providerFormName = ""
	a.providerFormBaseURL = ""
	a.providerFormAPI = "openai-completions"
	a.providerFormAPIKey = ""
	a.providerFormAdvanced = "{}"
	a.providerFormError = ""
	if s != nil {
		a.providerFormSessionID = s.ID
	} else {
		a.providerFormSessionID = ""
	}
	if id != "" {
		found := false
		for _, p := range a.providers {
			if p.ID == id {
				a.providerFormName = p.Name
				a.providerFormBaseURL = p.BaseURL
				a.providerFormAPI = p.API
				found = true
				break
			}
		}
		if !found {
			a.pendingToast = "Provider not found"
			return
		}
		root, _, _, err := readJSONObject(a.modelsPath())
		if err != nil {
			a.pendingToast = "Models: " + err.Error()
			return
		}
		if providers := jsonObjectField(root, "providers"); providers != nil {
			if entry, ok := providers[id].(map[string]any); ok {
				a.providerFormAPIKey = jsonStringField(entry, "apiKey")
				a.providerFormAdvanced = advancedJSON(entry, providerFormHandled)
			}
		}
	}
	a.providerFormOpen = true
}

func (a *app) providerFormDialog(c *ui.Context) {
	p := a.pal()
	title := "Add provider"
	if !a.providerFormAdd {
		title = "Edit provider"
	}
	ui.Modal(c, &a.providerFormOpen, func() {
		ui.Text(c, title).FontSize(18).FontWeight(600).TextColor(p.text)
		ui.Text(c, "Stored in models.json. API keys saved under auth.json stay separate.").
			FontSize(11).TextColor(p.text4)
		ui.Column(c).FillWidth().Gap(8).Children(func() {
			a.formFieldLabel(c, "Provider ID", "letters, digits, ., _ and -")
			if a.providerFormAdd {
				a.formInput(c, &a.providerFormID, "Provider ID", "my-provider")
			} else {
				ui.Text(c, a.providerFormID).Font("monospace").FontSize(12).TextColor(p.text2)
			}
			a.formFieldLabel(c, "Display name", "shown in settings and the model menu")
			a.formInput(c, &a.providerFormName, "Display name", "My Provider")
			a.formFieldLabel(c, "Base URL", "OpenAI-compatible endpoint")
			a.formInput(c, &a.providerFormBaseURL, "Base URL", "https://example.com/v1")
			a.formFieldLabel(c, "API", "openai-completions, openai-responses, anthropic-messages")
			a.formInput(c, &a.providerFormAPI, "API", "openai-completions")
			a.formFieldLabel(c, "models.json API key", "optional; prefer the auth.json Set key button")
			key := ui.TextInputBase(c, &a.providerFormAPIKey).FillWidth().Height(32).Padding(0, 9).Radius(6).
				Border(1, p.lineStrong).Background(p.surface2).FontSize(12).TextColor(p.text).
				Placeholder("sk-… or $ENV_VAR").Label("models.json API key").Password()
			_ = key
			a.formFieldLabel(c, "Advanced JSON", "extra models.json provider keys")
			a.formTextArea(c, &a.providerFormAdvanced, "Advanced JSON", 5)
			if a.providerFormError != "" {
				ui.Text(c, a.providerFormError).FontSize(11).TextColor(p.bad)
			}
		})
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			if !a.providerFormAdd {
				if a.dangerButton(c, "Delete").Clicked() {
					id := a.providerFormID
					name := a.providerFormName
					a.providerFormOpen = false
					a.confirmDestructive("Delete provider?", "Delete "+name+" and all of its models from models.json. This cannot be undone.", "Delete provider", func() {
						if err := a.deleteProvider(id); err != nil {
							a.pendingToast = "Providers: " + err.Error()
						}
					})
				}
			}
			ui.Spacer(c)
			if a.button(c, "Cancel").Clicked() {
				a.providerFormOpen = false
			}
			if a.primaryButton(c, "Save provider", "check").Clicked() {
				a.finishProviderForm()
			}
		})
	})
}

func (a *app) finishProviderForm() {
	id := strings.TrimSpace(a.providerFormID)
	if id == "" || id != a.providerFormID {
		a.providerFormError = "provider ID is required and must not have surrounding spaces"
		return
	}
	if a.providerFormAdd && !validProviderID(id) {
		a.providerFormError = "provider ID must not contain spaces or slashes"
		return
	}
	api := strings.TrimSpace(a.providerFormAPI)
	if api == "" {
		a.providerFormError = "API is required"
		return
	}
	name := strings.TrimSpace(a.providerFormName)
	if name == "" {
		name = id
	}
	s := a.providerFormSession()
	if err := a.host.validateConfigReload(a); err != nil {
		a.providerFormError = err.Error()
		return
	}
	path := a.modelsPath()
	root, previous, existed, err := readJSONObject(path)
	if err != nil {
		a.providerFormError = err.Error()
		return
	}
	providers := jsonObjectField(root, "providers")
	if providers == nil {
		providers = map[string]any{}
	}
	entry, _ := providers[id].(map[string]any)
	if a.providerFormAdd && entry != nil {
		a.providerFormError = "a provider with this ID already exists"
		return
	}
	if entry == nil {
		entry = map[string]any{}
	}
	if err := applyAdvancedJSON(entry, providerFormHandled, a.providerFormAdvanced); err != nil {
		a.providerFormError = "Advanced JSON: " + err.Error()
		return
	}
	entry["name"] = name
	if baseURL := strings.TrimSpace(a.providerFormBaseURL); baseURL != "" {
		entry["baseUrl"] = baseURL
	} else {
		delete(entry, "baseUrl")
	}
	entry["api"] = api
	if key := strings.TrimSpace(a.providerFormAPIKey); key != "" {
		entry["apiKey"] = key
	} else {
		delete(entry, "apiKey")
	}
	providers[id] = entry
	root["providers"] = providers
	if err := writeJSONObject(path, root); err != nil {
		a.providerFormError = err.Error()
		return
	}
	if err := a.reloadConfig("Models reloaded"); err != nil {
		restoreConfigFile(path, previous, existed)
		a.providerFormError = "reload failed: " + err.Error()
		return
	}
	a.providerFormOpen = false
	a.pendingToast = "Saved provider · " + name
	if s != nil {
		a.refreshCapabilities(s)
	}
}

func (a *app) modelSpecInUse(spec string) bool {
	if a == nil {
		return false
	}
	if a.host != nil && a.host.defaultMdl == spec {
		return true
	}
	for _, s := range a.sessions {
		if s.Model == spec || strings.HasPrefix(s.Model, spec+"/") {
			return true
		}
	}
	return false
}

func (a *app) providerInUse(id string) bool {
	if a == nil {
		return false
	}
	prefix := id + "/"
	if a.host != nil && strings.HasPrefix(a.host.defaultMdl, prefix) {
		return true
	}
	for _, s := range a.sessions {
		if strings.HasPrefix(s.Model, prefix) {
			return true
		}
	}
	return false
}

func (a *app) deleteProvider(id string) error {
	if a == nil || a.host == nil || id == "" {
		return errors.New("provider ID is required")
	}
	if a.providerInUse(id) {
		return errors.New("switch sessions and the default model away from this provider first")
	}
	if err := a.host.validateConfigReload(a); err != nil {
		return err
	}
	path := a.modelsPath()
	root, previous, existed, err := readJSONObject(path)
	if err != nil {
		return err
	}
	providers := jsonObjectField(root, "providers")
	if providers == nil {
		return errors.New("provider not found")
	}
	if _, ok := providers[id]; !ok {
		return errors.New("provider not found")
	}
	delete(providers, id)
	root["providers"] = providers
	if err := writeJSONObject(path, root); err != nil {
		return err
	}
	if err := a.reloadConfig("Models reloaded"); err != nil {
		restoreConfigFile(path, previous, existed)
		return err
	}
	a.pendingToast = "Deleted provider · " + id
	return nil
}

func (a *app) openModelForm(s *session, providerID string, mdl model) {
	if a == nil || a.host == nil {
		return
	}
	a.modelFormProvider = providerID
	a.modelFormAdd = mdl.ID == ""
	a.modelFormID = mdl.ID
	a.modelFormName = mdl.Name
	a.modelFormContext = ""
	a.modelFormMaxOut = ""
	a.modelFormReasoning = mdl.Reasoning
	a.modelFormImage = mdl.Image
	a.modelFormError = ""
	a.modelFormAdvanced = "{}"
	if !a.modelFormAdd {
		if entry := a.modelConfigEntry(providerID, mdl.ID); entry != nil {
			a.modelFormAdvanced = advancedJSON(entry, modelFormHandled)
		}
	}
	if a.modelFormAdd {
		a.modelFormContext = "128000"
		a.modelFormMaxOut = "16384"
	} else {
		a.modelFormContext = strconv.Itoa(mdl.Context)
		a.modelFormMaxOut = strconv.Itoa(mdl.MaxOut)
	}
	if s != nil {
		a.modelFormSessionID = s.ID
	} else {
		a.modelFormSessionID = ""
	}
	a.modelFormOpen = true
}

func (a *app) modelConfigEntry(providerID, modelID string) map[string]any {
	if a == nil || a.host == nil {
		return nil
	}
	root, _, _, err := readJSONObject(a.modelsPath())
	if err != nil {
		return nil
	}
	providers := jsonObjectField(root, "providers")
	providerEntry, _ := providers[providerID].(map[string]any)
	if providerEntry == nil {
		return nil
	}
	models, _ := providerEntry["models"].([]any)
	for _, item := range models {
		entry, _ := item.(map[string]any)
		if entry != nil && jsonStringField(entry, "id") == modelID {
			return entry
		}
	}
	return nil
}

func (a *app) modelFormDialog(c *ui.Context) {
	p := a.pal()
	title := "Add model"
	if !a.modelFormAdd {
		title = "Edit model"
	}
	ui.Modal(c, &a.modelFormOpen, func() {
		ui.Text(c, title).FontSize(18).FontWeight(600).TextColor(p.text)
		ui.Text(c, "Provider · "+a.modelFormProvider).Font("monospace").FontSize(10).TextColor(p.text4)
		ui.Column(c).FillWidth().Gap(8).Children(func() {
			a.formFieldLabel(c, "Model ID", "the id sent to the provider")
			if a.modelFormAdd {
				a.formInput(c, &a.modelFormID, "Model ID", "my-model")
			} else {
				ui.Text(c, a.modelFormID).Font("monospace").FontSize(12).TextColor(p.text2)
			}
			a.formFieldLabel(c, "Display name", "shown in the model menu")
			a.formInput(c, &a.modelFormName, "Display name", "My Model")
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				ui.Column(c).Grow(1).Gap(4).Children(func() {
					a.formFieldLabel(c, "Context", "tokens")
					a.formInput(c, &a.modelFormContext, "Context window", "128000")
				})
				ui.Column(c).Grow(1).Gap(4).Children(func() {
					a.formFieldLabel(c, "Max output", "tokens")
					a.formInput(c, &a.modelFormMaxOut, "Max output", "16384")
				})
			})
			ui.Row(c).Gap(12).Children(func() {
				if a.checkbox(c, a.modelFormReasoning, "Model supports reasoning").Clicked() {
					a.modelFormReasoning = !a.modelFormReasoning
				}
				ui.Text(c, "Reasoning").FontSize(12).TextColor(p.text3)
				if a.checkbox(c, a.modelFormImage, "Model accepts images").Clicked() {
					a.modelFormImage = !a.modelFormImage
				}
				ui.Text(c, "Image input").FontSize(12).TextColor(p.text3)
			})
			a.formFieldLabel(c, "Advanced JSON", "extra models.json model keys: cost, compat, headers")
			a.formTextArea(c, &a.modelFormAdvanced, "Advanced JSON", 5)
			if a.modelFormError != "" {
				ui.Text(c, a.modelFormError).FontSize(11).TextColor(p.bad)
			}
		})
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			if !a.modelFormAdd {
				if a.dangerButton(c, "Delete").Clicked() {
					providerID, modelID := a.modelFormProvider, a.modelFormID
					name := a.modelFormName
					a.modelFormOpen = false
					a.confirmDestructive("Delete model?", "Delete "+name+" from "+providerID+". This cannot be undone.", "Delete model", func() {
						if err := a.deleteModel(providerID, modelID); err != nil {
							a.pendingToast = "Models: " + err.Error()
						}
					})
				}
			}
			ui.Spacer(c)
			if a.button(c, "Cancel").Clicked() {
				a.modelFormOpen = false
			}
			if a.primaryButton(c, "Save model", "check").Clicked() {
				a.finishModelForm()
			}
		})
	})
}

func (a *app) finishModelForm() {
	providerID := a.modelFormProvider
	if providerID == "" {
		a.modelFormError = "provider is missing"
		return
	}
	id := strings.TrimSpace(a.modelFormID)
	if id == "" || id != a.modelFormID {
		a.modelFormError = "model ID is required and must not have surrounding spaces"
		return
	}
	if a.modelFormAdd && !validModelID(id) {
		a.modelFormError = "model ID must not contain spaces"
		return
	}
	contextWindow, err := strconv.Atoi(strings.TrimSpace(a.modelFormContext))
	if err != nil || contextWindow <= 0 {
		a.modelFormError = "context window must be a positive number"
		return
	}
	maxOut, err := strconv.Atoi(strings.TrimSpace(a.modelFormMaxOut))
	if err != nil || maxOut <= 0 {
		a.modelFormError = "max output must be a positive number"
		return
	}
	name := strings.TrimSpace(a.modelFormName)
	if name == "" {
		name = id
	}
	s := a.modelFormSession()
	if err := a.host.validateConfigReload(a); err != nil {
		a.modelFormError = err.Error()
		return
	}
	path := a.modelsPath()
	root, previous, existed, err := readJSONObject(path)
	if err != nil {
		a.modelFormError = err.Error()
		return
	}
	providers := jsonObjectField(root, "providers")
	if providers == nil {
		a.modelFormError = "provider not found"
		return
	}
	providerEntry, _ := providers[providerID].(map[string]any)
	if providerEntry == nil {
		a.modelFormError = "provider not found in models.json"
		return
	}
	models, _ := providerEntry["models"].([]any)
	index := -1
	for i, item := range models {
		entry, _ := item.(map[string]any)
		if entry != nil && jsonStringField(entry, "id") == id {
			index = i
			break
		}
	}
	if a.modelFormAdd && index >= 0 {
		a.modelFormError = "a model with this ID already exists"
		return
	}
	var entry map[string]any
	if index >= 0 {
		entry, _ = models[index].(map[string]any)
	}
	if entry == nil {
		entry = map[string]any{}
	}
	if err := applyAdvancedJSON(entry, modelFormHandled, a.modelFormAdvanced); err != nil {
		a.modelFormError = "Advanced JSON: " + err.Error()
		return
	}
	entry["id"] = id
	entry["name"] = name
	entry["contextWindow"] = contextWindow
	entry["maxTokens"] = maxOut
	if a.modelFormReasoning {
		entry["reasoning"] = true
	} else {
		delete(entry, "reasoning")
	}
	inputs := jsonStringList(entry["input"])
	hasImage := false
	for _, input := range inputs {
		hasImage = hasImage || input == "image"
	}
	if a.modelFormImage {
		if !containsValue(inputs, "text") {
			inputs = append([]string{"text"}, inputs...)
		}
		if !hasImage {
			inputs = append(inputs, "image")
		}
	} else {
		filtered := inputs[:0]
		for _, input := range inputs {
			if input != "image" {
				filtered = append(filtered, input)
			}
		}
		inputs = filtered
	}
	if len(inputs) > 0 {
		if !containsValue(inputs, "text") && !containsValue(inputs, "image") {
			inputs = append([]string{"text"}, inputs...)
		}
		entry["input"] = inputs
	} else {
		delete(entry, "input")
	}
	if index >= 0 {
		models[index] = entry
	} else {
		models = append(models, entry)
	}
	providerEntry["models"] = models
	providers[providerID] = providerEntry
	root["providers"] = providers
	if err := writeJSONObject(path, root); err != nil {
		a.modelFormError = err.Error()
		return
	}
	if err := a.reloadConfig("Models reloaded"); err != nil {
		restoreConfigFile(path, previous, existed)
		a.modelFormError = "reload failed: " + err.Error()
		return
	}
	a.modelFormOpen = false
	a.pendingToast = "Saved model · " + name
	if s != nil {
		a.refreshCapabilities(s)
	}
}

func containsValue(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func (a *app) deleteModel(providerID, modelID string) error {
	spec := providerID + "/" + modelID
	if a.modelSpecInUse(spec) {
		return errors.New("switch sessions and the default model away from this model first")
	}
	if err := a.host.validateConfigReload(a); err != nil {
		return err
	}
	path := a.modelsPath()
	root, previous, existed, err := readJSONObject(path)
	if err != nil {
		return err
	}
	providers := jsonObjectField(root, "providers")
	if providers == nil {
		return errors.New("provider not found")
	}
	providerEntry, _ := providers[providerID].(map[string]any)
	if providerEntry == nil {
		return errors.New("provider not found")
	}
	models, _ := providerEntry["models"].([]any)
	found := false
	out := make([]any, 0, len(models))
	for _, item := range models {
		entry, _ := item.(map[string]any)
		if entry != nil && jsonStringField(entry, "id") == modelID {
			found = true
			continue
		}
		out = append(out, item)
	}
	if !found {
		return errors.New("model not found")
	}
	providerEntry["models"] = out
	providers[providerID] = providerEntry
	root["providers"] = providers
	if err := writeJSONObject(path, root); err != nil {
		return err
	}
	if err := a.reloadConfig("Models reloaded"); err != nil {
		restoreConfigFile(path, previous, existed)
		return err
	}
	a.pendingToast = "Deleted model · " + spec
	return nil
}
