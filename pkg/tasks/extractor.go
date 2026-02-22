package tasks

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lucas-stellet/playbookd"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/tools"
	"github.com/sipeed/picoclaw/pkg/utils"
)

// PlaybookExtractor creates playbooks from completed task sessions in the background.
type PlaybookExtractor struct{}

// MaybeExtract is a no-op. Playbook extraction now only happens on the explicit
// task-finished signal via MaybeExtractOnFinish.
func (pe *PlaybookExtractor) MaybeExtract(
	pm *playbookd.PlaybookManager,
	provider providers.LLMProvider, model string, maxTokens int,
	sessions SessionProvider, sessionKey, finalResponse, taskDescription string, iterations int,
) {
	// No-op: extraction only on task-finished signal
}

// MaybeExtractOnFinish extracts a playbook when a task-finished signal is received.
func (pe *PlaybookExtractor) MaybeExtractOnFinish(
	pm *playbookd.PlaybookManager,
	provider providers.LLMProvider, model string, maxTokens int,
	sessions SessionProvider, sessionKey, botMessage, taskDescription string,
) {
	if taskDescription == "" || pm == nil {
		logger.DebugCF("playbook", "MaybeExtractOnFinish skipped (no task description or playbook manager)", map[string]any{
			"session_key":      sessionKey,
			"task_description": taskDescription,
			"has_manager":      pm != nil,
		})
		return
	}
	// Use bot message as session context; fall back to session history if available
	sessionContext := botMessage
	if sessionContext == "" {
		history := sessions.GetHistory(sessionKey)
		for i := len(history) - 1; i >= 0; i-- {
			if history[i].Role == "assistant" && history[i].Content != "" {
				sessionContext = history[i].Content
				break
			}
		}
	}
	logger.InfoCF("playbook", "Starting playbook extraction from task-finished signal", map[string]any{
		"session_key":      sessionKey,
		"task_description": taskDescription,
		"context_len":      len(sessionContext),
		"has_context":      sessionContext != "",
	})
	go pe.extract(pm, provider, model, maxTokens, sessions, sessionKey, sessionContext, taskDescription)
}

// extract runs a background LLM call to distill a completed task session into a reusable playbook.
func (pe *PlaybookExtractor) extract(
	pm *playbookd.PlaybookManager,
	provider providers.LLMProvider, model string, maxTokens int,
	sessions SessionProvider, sessionKey, finalResponse, taskDescription string,
) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Build session log from history if available; otherwise use finalResponse directly
	sessionLog := ""
	history := sessions.GetHistory(sessionKey)
	if len(history) > 0 {
		sessionLog = buildSessionLog(history)
	}

	if sessionLog == "" && finalResponse == "" {
		logger.DebugCF("playbook", "Playbook extraction skipped (no session log and no final response)", map[string]any{
			"session_key": sessionKey,
		})
		return
	}

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
	playbookTools.Register(NewPlaybookCreateTool(pm))

	_, err := tools.RunToolLoop(ctx, tools.ToolLoopConfig{
		Provider:      provider,
		Model:         model,
		Tools:         playbookTools,
		MaxIterations: 3,
		LLMOptions: map[string]any{
			"max_tokens":  maxTokens,
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
