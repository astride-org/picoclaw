package tasks

import (
	"github.com/lucas-stellet/playbookd"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// SessionProvider provides access to session history.
type SessionProvider interface {
	GetHistory(sessionKey string) []providers.Message
}

// TaskManager manages task mode lifecycle: tool registration, prompt building,
// task-finished handling, and playbook extraction.
type TaskManager interface {
	RegisterTools(registry *tools.ToolRegistry, agentID string)
	BuildPromptSection(taskMode bool, taskDescription string) string
	HandleTaskFinished(provider providers.LLMProvider, model string, maxTokens int,
		sessions SessionProvider, sessionKey, botMessage, taskDescription string) string
	MaybeExtract(provider providers.LLMProvider, model string, maxTokens int,
		sessions SessionProvider, sessionKey, finalResponse, taskDescription string, iterations int)
	Close() error
}

// DefaultTaskManager implements TaskManager backed by a playbookd.PlaybookManager.
type DefaultTaskManager struct {
	playbookManager *playbookd.PlaybookManager
	extractor       *PlaybookExtractor
}

// NewTaskManager creates a TaskManager for the given workspace.
// If playbook initialization fails, a noopTaskManager is returned so the
// agent can continue without playbook support.
func NewTaskManager(workspace string) TaskManager {
	pm, err := initPlaybookManager(workspace)
	if err != nil {
		logger.WarnCF("tasks", "Playbook init failed, task mode will be limited", map[string]any{
			"error": err.Error(),
		})
		return &noopTaskManager{}
	}
	return &DefaultTaskManager{
		playbookManager: pm,
		extractor:       &PlaybookExtractor{},
	}
}

func (tm *DefaultTaskManager) RegisterTools(registry *tools.ToolRegistry, agentID string) {
	registry.Register(NewPlaybookSearchTool(tm.playbookManager))
	registry.Register(NewPlaybookCreateTool(tm.playbookManager))
	registry.Register(NewPlaybookRecordTool(tm.playbookManager, agentID))
	registry.Register(NewPlaybookListTool(tm.playbookManager))
}

func (tm *DefaultTaskManager) BuildPromptSection(taskMode bool, taskDescription string) string {
	return buildPlaybookContext(tm.playbookManager, taskMode, taskDescription)
}

func (tm *DefaultTaskManager) HandleTaskFinished(
	provider providers.LLMProvider, model string, maxTokens int,
	sessions SessionProvider, sessionKey, botMessage, taskDescription string,
) string {
	tm.extractor.MaybeExtractOnFinish(
		tm.playbookManager, provider, model, maxTokens,
		sessions, sessionKey, botMessage, taskDescription,
	)
	return "Task finished. Extracting playbook from this session."
}

func (tm *DefaultTaskManager) MaybeExtract(
	provider providers.LLMProvider, model string, maxTokens int,
	sessions SessionProvider, sessionKey, finalResponse, taskDescription string, iterations int,
) {
	tm.extractor.MaybeExtract(
		tm.playbookManager, provider, model, maxTokens,
		sessions, sessionKey, finalResponse, taskDescription, iterations,
	)
}

func (tm *DefaultTaskManager) Close() error {
	if tm.playbookManager != nil {
		return tm.playbookManager.Close()
	}
	return nil
}

// noopTaskManager is a no-op implementation used when playbook init fails.
type noopTaskManager struct{}

func (n *noopTaskManager) RegisterTools(registry *tools.ToolRegistry, agentID string) {}
func (n *noopTaskManager) BuildPromptSection(taskMode bool, taskDescription string) string {
	return ""
}
func (n *noopTaskManager) HandleTaskFinished(
	provider providers.LLMProvider, model string, maxTokens int,
	sessions SessionProvider, sessionKey, botMessage, taskDescription string,
) string {
	return ""
}
func (n *noopTaskManager) MaybeExtract(
	provider providers.LLMProvider, model string, maxTokens int,
	sessions SessionProvider, sessionKey, finalResponse, taskDescription string, iterations int,
) {
}
func (n *noopTaskManager) Close() error { return nil }
