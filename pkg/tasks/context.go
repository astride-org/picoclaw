package tasks

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lucas-stellet/playbookd"
	"github.com/sipeed/picoclaw/pkg/logger"
)

// buildPlaybookContext searches for relevant playbooks and formats them for the system prompt.
func buildPlaybookContext(pm *playbookd.PlaybookManager, taskMode bool, taskDescription string) string {
	if !taskMode || pm == nil || taskDescription == "" {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("# Task Mode\n\n")
	sb.WriteString(fmt.Sprintf("**Task**: %s\n\n", taskDescription))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	results, err := pm.Search(ctx, playbookd.SearchQuery{
		Text:  taskDescription,
		Mode:  playbookd.SearchModeHybrid,
		Limit: 3,
	})
	if err != nil {
		logger.WarnCF("tasks", "Playbook search failed", map[string]any{
			"error": err.Error(),
		})
		sb.WriteString("(Playbook search failed. You can still create a new playbook with the `playbook_create` tool after completing this task.)\n")
		return sb.String()
	}

	if len(results) == 0 {
		sb.WriteString("No existing playbooks found for this task.\n\n")
		sb.WriteString("After completing this task, consider creating a playbook using the `playbook_create` tool so you can follow it next time.\n")
		return sb.String()
	}

	sb.WriteString("## Relevant Playbooks\n\n")
	for _, result := range results {
		pb := result.Playbook
		sb.WriteString(fmt.Sprintf("### %s (confidence: %.0f%%, v%d, score: %.2f)\n\n",
			pb.Name, pb.Confidence*100, pb.Version, result.Score))

		if pb.Description != "" {
			sb.WriteString(fmt.Sprintf("%s\n\n", pb.Description))
		}

		sb.WriteString("**Steps:**\n")
		for _, step := range pb.Steps {
			line := fmt.Sprintf("%d. %s", step.Order, step.Action)
			if step.Tool != "" {
				line += fmt.Sprintf(" (tool: %s)", step.Tool)
			}
			if step.Expected != "" {
				line += fmt.Sprintf(" → expect: %s", step.Expected)
			}
			sb.WriteString(line + "\n")
		}

		if len(pb.Lessons) > 0 {
			sb.WriteString("\n**Lessons learned:**\n")
			for _, lesson := range pb.Lessons {
				sb.WriteString(fmt.Sprintf("- %s\n", lesson.Content))
			}
		}

		sb.WriteString(fmt.Sprintf("\n**Playbook ID**: `%s`\n\n", pb.ID))
	}

	sb.WriteString("After completing the task, use `playbook_record` to record the execution outcome. This helps improve confidence scores and track lessons learned.\n")

	return sb.String()
}
