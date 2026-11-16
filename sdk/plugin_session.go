package sdk

import (
	"context"
	"fmt"
)

// AddTool registers a tool for this session. It is useful for tool sources
// discovered at session start, such as MCP servers. The tool becomes active on
// the next request and is recorded in the transcript's tool declaration.
func (s *Session) AddTool(t Tool) error {
	if t == nil {
		return fmt.Errorf("sdk: nil tool")
	}
	def := t.Definition()
	if def.Name == "" {
		return fmt.Errorf("sdk: tool name is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.disposed {
		return fmt.Errorf("session disposed")
	}
	for _, existing := range s.catalog {
		if existing.Name == def.Name {
			return fmt.Errorf("sdk: tool %q already exists", def.Name)
		}
	}
	tool := agentToolFromPlugin(t)
	s.tools = append(s.tools, tool)
	s.catalog = append(s.catalog, tool)
	return nil
}

// Commands returns the slash commands contributed by SDK plugins.
func (s *Session) Commands() []Command {
	if s == nil || s.services == nil || s.services.Registry == nil {
		return nil
	}
	return s.services.Registry.Commands()
}

// RunCommand executes a plugin command by name.
func (s *Session) RunCommand(ctx context.Context, name, args string) error {
	if s == nil || s.services == nil || s.services.Registry == nil {
		return fmt.Errorf("sdk: command %q is not available", name)
	}
	for _, command := range s.services.Registry.Commands() {
		if command.Name == name {
			if command.Run == nil {
				return fmt.Errorf("sdk: command %q has no handler", name)
			}
			return command.Run(ctx, s, args)
		}
	}
	return fmt.Errorf("sdk: command %q is not registered", name)
}

// UI returns the host-facing UI declarations contributed by plugins.
func (s *Session) UI() UIDeclaration {
	if s == nil || s.services == nil || s.services.Registry == nil {
		return UIDeclaration{}
	}
	return s.services.Registry.UI()
}

// Plugins returns the plugin registry attached to the session's services.
func (s *Session) Plugins() *Registry {
	if s == nil || s.services == nil {
		return nil
	}
	return s.services.Registry
}
