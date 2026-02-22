package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/tools"
	"github.com/sipeed/picoclaw/pkg/utils"
)

// PlaybookExtractor creates playbooks from completed task sessions in the background.
type PlaybookExtractor struct{}

// MaybeExtract checks if playbook extraction should run and spawns it in the background.
// It is a no-op when conditions are not met (no task mode, no playbook manager, trivial session).
func (pe *PlaybookExtractor) MaybeExtract(agent *AgentInstance, sessionKey, finalResponse, taskDescription string, iterations int) {
	if taskDescription == "" || agent.PlaybookManager == nil || iterations <= 1 {
		return
	}
	go pe.extract(agent, sessionKey, finalResponse, taskDescription)
}

// extract runs a background LLM call to distill a completed task session into a reusable playbook.
// It gives the LLM access only to playbook_create.
func (pe *PlaybookExtractor) extract(agent *AgentInstance, sessionKey, finalResponse, taskDescription string) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	history := agent.Sessions.GetHistory(sessionKey)
	if len(history) == 0 {
		return
	}

	sessionLog := buildSessionLog(history)

	prompt := fmt.Sprintf(`You just completed a task. Extract a reusable playbook from this session.

Task: %s

Final response: %s

Session log:
%s

Create a playbook using the playbook_create tool. Include:
- A clear name describing the type of problem solved
- A description of when this playbook applies
- The key steps that were taken (generalized, not specific to this instance)
- Appropriate tags and category for searchability

Generalize the steps so they can be reused for similar tasks in the future.`,
		taskDescription,
		utils.Truncate(finalResponse, 500),
		sessionLog,
	)

	playbookTools := tools.NewToolRegistry()
	playbookTools.Register(tools.NewPlaybookCreateTool(agent.PlaybookManager))

	_, err := tools.RunToolLoop(ctx, tools.ToolLoopConfig{
		Provider:      agent.Provider,
		Model:         agent.Model,
		Tools:         playbookTools,
		MaxIterations: 3,
		LLMOptions: map[string]any{
			"max_tokens":  agent.MaxTokens,
			"temperature": 0.3,
		},
	}, []providers.Message{
		{Role: "system", Content: "You are a playbook extraction agent. Your only job is to create a playbook from the completed task session using the playbook_create tool. Do not respond with text — just call the tool."},
		{Role: "user", Content: prompt},
	}, "system", "playbook-extraction")

	if err != nil {
		logger.WarnCF("playbook", "Playbook extraction failed", map[string]any{
			"error":       err.Error(),
			"session_key": sessionKey,
		})
		return
	}

	logger.InfoCF("playbook", "Playbook extracted from task session", map[string]any{
		"session_key":      sessionKey,
		"task_description": taskDescription,
	})
}

// buildSessionLog creates a condensed text representation of a session's message history.
func buildSessionLog(history []providers.Message) string {
	var sb strings.Builder
	for _, msg := range history {
		switch msg.Role {
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				for _, tc := range msg.ToolCalls {
					args := ""
					if tc.Function != nil {
						args = utils.Truncate(tc.Function.Arguments, 200)
					}
					fmt.Fprintf(&sb, "TOOL_CALL: %s(%s)\n", tc.Name, args)
				}
			} else if msg.Content != "" {
				fmt.Fprintf(&sb, "ASSISTANT: %s\n", utils.Truncate(msg.Content, 300))
			}
		case "tool":
			fmt.Fprintf(&sb, "TOOL_RESULT: %s\n", utils.Truncate(msg.Content, 200))
		case "user":
			fmt.Fprintf(&sb, "USER: %s\n", utils.Truncate(msg.Content, 200))
		}
	}
	return sb.String()
}
