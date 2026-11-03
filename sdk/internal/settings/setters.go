package settings

import (
	"encoding/json"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

// setGlobalList applies Pi's nil/empty distinction: nil removes the key and
// an explicitly empty slice persists [].
func (m *Manager) setGlobalList(key string, values []string) error {
	if values == nil {
		return m.mutateGlobal(func(layer map[string]json.RawMessage) { delete(layer, key) })
	}
	return m.setGlobalValue(key, values)
}

func (m *Manager) setProjectList(key string, values []string) error {
	if values == nil {
		return m.mutateProject(func(layer map[string]json.RawMessage) { delete(layer, key) })
	}
	return m.setProjectValue(key, values)
}

func (m *Manager) setGlobalValue(key string, value any) error {
	encoded, err := marshalJSONValue(value)
	if err != nil {
		return err
	}
	return m.mutateGlobal(func(global map[string]json.RawMessage) {
		global[key] = encoded
	})
}

func (m *Manager) setProjectValue(key string, value any) error {
	encoded, err := marshalJSONValue(value)
	if err != nil {
		return err
	}
	return m.mutateProject(func(project map[string]json.RawMessage) {
		project[key] = encoded
	})
}

func (m *Manager) updateGlobalObject(key string, fn func(map[string]json.RawMessage)) error {
	return m.mutateGlobal(func(global map[string]json.RawMessage) {
		object := objectForSpread(global[key])
		fn(object)
		encoded, err := marshalJSONValue(object)
		if err == nil {
			global[key] = encoded
		}
	})
}

func (m *Manager) updateProjectObject(key string, fn func(map[string]json.RawMessage)) error {
	return m.mutateProject(func(project map[string]json.RawMessage) {
		object := objectForSpread(project[key])
		fn(object)
		encoded, err := marshalJSONValue(object)
		if err == nil {
			project[key] = encoded
		}
	})
}

// SetDefaultProvider writes defaultProvider.
func (m *Manager) SetDefaultProvider(provider string) error {
	return m.setGlobalValue("defaultProvider", provider)
}

// SetDefaultModel writes defaultModel.
func (m *Manager) SetDefaultModel(model string) error {
	return m.setGlobalValue("defaultModel", model)
}

// SetDefaultModelAndProvider writes both keys in one update.
func (m *Manager) SetDefaultModelAndProvider(provider, model string) error {
	providerJSON, err := marshalJSONValue(provider)
	if err != nil {
		return err
	}
	modelJSON, err := marshalJSONValue(model)
	if err != nil {
		return err
	}
	return m.mutateGlobalOrdered([]string{"defaultProvider", "defaultModel"}, func(global map[string]json.RawMessage) {
		global["defaultProvider"] = providerJSON
		global["defaultModel"] = modelJSON
	})
}

// SetDefaultThinkingLevel writes defaultThinkingLevel.
func (m *Manager) SetDefaultThinkingLevel(level ai.ThinkingLevel) error {
	return m.setGlobalValue("defaultThinkingLevel", string(level))
}

// SetModelThinkingLevel writes one modelThinkingLevels entry.
func (m *Manager) SetModelThinkingLevel(provider, modelID string, level ai.ThinkingLevel) error {
	return m.updateGlobalObject("modelThinkingLevels", func(levels map[string]json.RawMessage) {
		encoded, _ := marshalJSONValue(string(level))
		levels[provider+"/"+modelID] = encoded
	})
}

// RemoveModelThinkingLevel deletes one modelThinkingLevels entry, removing
// the object when it was the last one.
func (m *Manager) RemoveModelThinkingLevel(provider, modelID string) error {
	return m.mutateGlobal(func(global map[string]json.RawMessage) {
		levels, _ := decodeRawObject(global["modelThinkingLevels"])
		if levels == nil {
			return
		}
		delete(levels, provider+"/"+modelID)
		if len(levels) == 0 {
			delete(global, "modelThinkingLevels")
			return
		}
		encoded, err := marshalJSONValue(levels)
		if err == nil {
			global["modelThinkingLevels"] = encoded
		}
	})
}

// SetEnabledModels writes enabledModels, including an explicit empty list.
func (m *Manager) SetEnabledModels(patterns []string) error {
	return m.setGlobalList("enabledModels", patterns)
}

// SetDefaultTools writes defaultTools, including an explicit empty list.
func (m *Manager) SetDefaultTools(tools []string) error {
	return m.setGlobalList("defaultTools", tools)
}

// SetCompactionEnabled writes compaction.enabled.
func (m *Manager) SetCompactionEnabled(enabled bool) error {
	return m.updateGlobalObject("compaction", func(compaction map[string]json.RawMessage) {
		encoded, _ := marshalJSONValue(enabled)
		compaction["enabled"] = encoded
	})
}

// SetCompactionReserveTokens writes compaction.reserveTokens.
func (m *Manager) SetCompactionReserveTokens(tokens int) error {
	return m.updateGlobalObject("compaction", func(compaction map[string]json.RawMessage) {
		encoded, _ := marshalJSONValue(tokens)
		compaction["reserveTokens"] = encoded
	})
}

// SetCompactionKeepRecentTokens writes compaction.keepRecentTokens.
func (m *Manager) SetCompactionKeepRecentTokens(tokens int) error {
	return m.updateGlobalObject("compaction", func(compaction map[string]json.RawMessage) {
		encoded, _ := marshalJSONValue(tokens)
		compaction["keepRecentTokens"] = encoded
	})
}

// SetRetryEnabled writes retry.enabled.
func (m *Manager) SetRetryEnabled(enabled bool) error {
	return m.updateGlobalObject("retry", func(retry map[string]json.RawMessage) {
		encoded, _ := marshalJSONValue(enabled)
		retry["enabled"] = encoded
	})
}

// SetRetryMaxRetries writes retry.maxRetries.
func (m *Manager) SetRetryMaxRetries(retries int) error {
	return m.updateGlobalObject("retry", func(retry map[string]json.RawMessage) {
		encoded, _ := marshalJSONValue(retries)
		retry["maxRetries"] = encoded
	})
}

// SetRetryBaseDelayMs writes retry.baseDelayMs.
func (m *Manager) SetRetryBaseDelayMs(delayMs int) error {
	return m.updateGlobalObject("retry", func(retry map[string]json.RawMessage) {
		encoded, _ := marshalJSONValue(delayMs)
		retry["baseDelayMs"] = encoded
	})
}

// SetRetryMaxAgentDelayMs writes retry.maxAgentDelayMs.
func (m *Manager) SetRetryMaxAgentDelayMs(delayMs int) error {
	return m.updateGlobalObject("retry", func(retry map[string]json.RawMessage) {
		encoded, _ := marshalJSONValue(delayMs)
		retry["maxAgentDelayMs"] = encoded
	})
}

// SetBranchSummaryReserveTokens writes branchSummary.reserveTokens.
func (m *Manager) SetBranchSummaryReserveTokens(tokens int) error {
	return m.updateGlobalObject("branchSummary", func(branch map[string]json.RawMessage) {
		encoded, _ := marshalJSONValue(tokens)
		branch["reserveTokens"] = encoded
	})
}

// SetBranchSummarySkipPrompt writes branchSummary.skipPrompt.
func (m *Manager) SetBranchSummarySkipPrompt(skip bool) error {
	return m.updateGlobalObject("branchSummary", func(branch map[string]json.RawMessage) {
		encoded, _ := marshalJSONValue(skip)
		branch["skipPrompt"] = encoded
	})
}

// SetCacheWarmingMode writes cacheWarming.
func (m *Manager) SetCacheWarmingMode(mode string) error {
	return m.setGlobalValue("cacheWarming", mode)
}

// SetSessionDir writes sessionDir.
func (m *Manager) SetSessionDir(dir string) error {
	return m.setGlobalValue("sessionDir", dir)
}

// SetShellPath writes shellPath.
func (m *Manager) SetShellPath(path string) error {
	return m.setGlobalValue("shellPath", path)
}

// SetShellCommandPrefix writes shellCommandPrefix.
func (m *Manager) SetShellCommandPrefix(prefix string) error {
	return m.setGlobalValue("shellCommandPrefix", prefix)
}

// SetNpmCommand writes npmCommand, including an explicit empty list.
func (m *Manager) SetNpmCommand(command []string) error {
	return m.setGlobalList("npmCommand", command)
}

// SetHTTPProxy writes httpProxy.
func (m *Manager) SetHTTPProxy(proxy string) error {
	return m.setGlobalValue("httpProxy", proxy)
}

// SetSkillPaths writes skills to the global layer.
func (m *Manager) SetSkillPaths(paths []string) error {
	return m.setGlobalList("skills", paths)
}

// SetProjectSkillPaths writes skills to the project layer.
func (m *Manager) SetProjectSkillPaths(paths []string) error {
	return m.setProjectList("skills", paths)
}

// SetPromptTemplatePaths writes prompts to the global layer.
func (m *Manager) SetPromptTemplatePaths(paths []string) error {
	return m.setGlobalList("prompts", paths)
}

// SetProjectPromptTemplatePaths writes prompts to the project layer.
func (m *Manager) SetProjectPromptTemplatePaths(paths []string) error {
	return m.setProjectList("prompts", paths)
}

// SetThemePaths writes themes to the global layer.
func (m *Manager) SetThemePaths(paths []string) error {
	return m.setGlobalList("themes", paths)
}

// SetProjectThemePaths writes themes to the project layer.
func (m *Manager) SetProjectThemePaths(paths []string) error {
	return m.setProjectList("themes", paths)
}

// SetExtensionPaths writes extensions to the global layer.
func (m *Manager) SetExtensionPaths(paths []string) error {
	return m.setGlobalList("extensions", paths)
}

// SetProjectExtensionPaths writes extensions to the project layer.
func (m *Manager) SetProjectExtensionPaths(paths []string) error {
	return m.setProjectList("extensions", paths)
}

// SetTransport writes transport.
func (m *Manager) SetTransport(transport string) error {
	return m.setGlobalValue("transport", transport)
}

// SetSteeringMode writes steeringMode.
func (m *Manager) SetSteeringMode(mode string) error {
	return m.setGlobalValue("steeringMode", mode)
}

// SetFollowUpMode writes followUpMode.
func (m *Manager) SetFollowUpMode(mode string) error {
	return m.setGlobalValue("followUpMode", mode)
}

// SetTheme writes theme.
func (m *Manager) SetTheme(theme string) error {
	return m.setGlobalValue("theme", theme)
}

// SetHideThinkingBlock writes hideThinkingBlock.
func (m *Manager) SetHideThinkingBlock(hide bool) error {
	return m.setGlobalValue("hideThinkingBlock", hide)
}

// SetShowCacheMissNotices writes showCacheMissNotices.
func (m *Manager) SetShowCacheMissNotices(show bool) error {
	return m.setGlobalValue("showCacheMissNotices", show)
}

// SetShowImages writes terminal.showImages.
func (m *Manager) SetShowImages(show bool) error {
	return m.updateGlobalObject("terminal", func(terminal map[string]json.RawMessage) {
		terminal["showImages"] = mustJSON(show)
	})
}

// SetImageWidthCells writes terminal.imageWidthCells, clamped to at least 1
// as Pi does.
func (m *Manager) SetImageWidthCells(width int) error {
	if width < 1 {
		width = 1
	}
	return m.updateGlobalObject("terminal", func(terminal map[string]json.RawMessage) {
		terminal["imageWidthCells"] = mustJSON(width)
	})
}

// SetClearOnShrink writes terminal.clearOnShrink.
func (m *Manager) SetClearOnShrink(enabled bool) error {
	return m.updateGlobalObject("terminal", func(terminal map[string]json.RawMessage) {
		terminal["clearOnShrink"] = mustJSON(enabled)
	})
}

// SetShowTerminalProgress writes terminal.showTerminalProgress.
func (m *Manager) SetShowTerminalProgress(enabled bool) error {
	return m.updateGlobalObject("terminal", func(terminal map[string]json.RawMessage) {
		terminal["showTerminalProgress"] = mustJSON(enabled)
	})
}

// SetImageAutoResize writes images.autoResize.
func (m *Manager) SetImageAutoResize(enabled bool) error {
	return m.updateGlobalObject("images", func(images map[string]json.RawMessage) {
		images["autoResize"] = mustJSON(enabled)
	})
}

// SetBlockImages writes images.blockImages.
func (m *Manager) SetBlockImages(block bool) error {
	return m.updateGlobalObject("images", func(images map[string]json.RawMessage) {
		images["blockImages"] = mustJSON(block)
	})
}

func mustJSON(value any) json.RawMessage {
	encoded, _ := marshalJSONValue(value)
	return encoded
}

// SetEnableSkillCommands writes enableSkillCommands.
func (m *Manager) SetEnableSkillCommands(enabled bool) error {
	return m.setGlobalValue("enableSkillCommands", enabled)
}

// SetDoubleEscapeAction writes doubleEscapeAction.
func (m *Manager) SetDoubleEscapeAction(action string) error {
	return m.setGlobalValue("doubleEscapeAction", action)
}

// SetTreeFilterMode writes treeFilterMode.
func (m *Manager) SetTreeFilterMode(mode string) error {
	return m.setGlobalValue("treeFilterMode", mode)
}

// SetDefaultProjectTrust writes defaultProjectTrust.
func (m *Manager) SetDefaultProjectTrust(trust DefaultProjectTrust) error {
	return m.setGlobalValue("defaultProjectTrust", string(trust))
}

// SetLastChangelogVersion writes lastChangelogVersion.
func (m *Manager) SetLastChangelogVersion(version string) error {
	return m.setGlobalValue("lastChangelogVersion", version)
}

// SetDeviceID writes deviceId.
func (m *Manager) SetDeviceID(id string) error {
	return m.setGlobalValue("deviceId", id)
}

// SetCodemodeMode writes codemode.mode.
func (m *Manager) SetCodemodeMode(mode string) error {
	return m.updateGlobalObject("codemode", func(codemode map[string]json.RawMessage) {
		encoded, _ := marshalJSONValue(mode)
		codemode["mode"] = encoded
	})
}

// SetCodemodeInlineBudget writes codemode.inlineBudget.
func (m *Manager) SetCodemodeInlineBudget(tokens int) error {
	return m.updateGlobalObject("codemode", func(codemode map[string]json.RawMessage) {
		encoded, _ := marshalJSONValue(tokens)
		codemode["inlineBudget"] = encoded
	})
}

// SetWarnings writes warnings, replacing the object as Pi's setter does.
func (m *Manager) SetWarnings(warnings WarningsSettings) error {
	return m.mutateGlobal(func(global map[string]json.RawMessage) {
		extraUsage := false
		if warnings.AnthropicExtraUsage != nil {
			extraUsage = *warnings.AnthropicExtraUsage
		}
		global["warnings"] = mustJSON(map[string]any{"anthropicExtraUsage": extraUsage})
	})
}
