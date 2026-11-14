package main

import "github.com/xiaokuang95/nulo/sdk"

type skillInfo struct {
	Name        string
	Description string
	Path        string
	Source      string
	Enabled     bool
}

func (a *app) refreshSkillInfo(s *session) {
	if a == nil || a.host == nil || s == nil || !s.Real {
		return
	}
	all := sdk.ListSkills(a.host.agentDir, s.CWD, a.host.sessionTrusted(s))
	out := make([]skillInfo, 0, len(all))
	for _, skill := range all {
		out = append(out, skillInfo{
			Name: skill.Name, Description: skill.Description, Path: skill.Path,
			Source: skill.Source, Enabled: !a.host.disabledSkills[skill.Name],
		})
	}
	s.SkillAll = out
}

func (a *app) refreshCapabilities(s *session) {
	a.refreshMCPInfo(s)
	a.refreshSkillInfo(s)
}

func (a *app) toggleSkill(s *session, name string, enabled bool) {
	if a == nil || a.host == nil || s == nil {
		return
	}
	if s.Status == "running" || s.Status == "awaiting" {
		a.pendingToast = "Stop the current turn before changing skills"
		return
	}
	if err := a.host.validateConfigReload(a); err != nil {
		a.pendingToast = "Skills: " + err.Error()
		return
	}
	wasDisabled := a.host.disabledSkills[name]
	if enabled {
		delete(a.host.disabledSkills, name)
	} else {
		a.host.disabledSkills[name] = true
	}
	if err := a.host.savePreferences(a); err != nil {
		if wasDisabled {
			a.host.disabledSkills[name] = true
		} else {
			delete(a.host.disabledSkills, name)
		}
		a.pendingToast = "Skills: " + err.Error()
		return
	}
	if err := a.reloadConfig("Skills reloaded"); err != nil {
		if wasDisabled {
			a.host.disabledSkills[name] = true
		} else {
			delete(a.host.disabledSkills, name)
		}
		_ = a.host.savePreferences(a)
		a.pendingToast = "Skills: " + err.Error()
		return
	}
	a.refreshCapabilities(s)
	if enabled {
		a.pendingToast = "Enabled skill · " + name
	} else {
		a.pendingToast = "Disabled skill · " + name
	}
}
