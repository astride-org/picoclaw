package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lucas-stellet/playbookd"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// --- playbook_search ---

type PlaybookSearchTool struct {
	manager *playbookd.PlaybookManager
}

func NewPlaybookSearchTool(manager *playbookd.PlaybookManager) *PlaybookSearchTool {
	return &PlaybookSearchTool{manager: manager}
}

func (t *PlaybookSearchTool) Name() string { return "playbook_search" }

func (t *PlaybookSearchTool) Description() string {
	return "Search for existing playbooks by query text (BM25). Returns matching playbooks with steps, confidence scores, and lessons learned."
}

func (t *PlaybookSearchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Search query to find relevant playbooks",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Maximum number of results (default: 5)",
			},
		},
		"required": []string{"query"},
	}
}

func (t *PlaybookSearchTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	query, _ := args["query"].(string)
	if query == "" {
		return tools.ErrorResult("query parameter is required")
	}

	limit := 5
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	results, err := t.manager.Search(ctx, playbookd.SearchQuery{
		Text:  query,
		Mode:  playbookd.SearchModeHybrid,
		Limit: limit,
	})
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("playbook search failed: %v", err))
	}

	if len(results) == 0 {
		return tools.SilentResult("No playbooks found matching the query.")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d playbook(s):\n\n", len(results)))
	for _, r := range results {
		pb := r.Playbook
		sb.WriteString(fmt.Sprintf("## %s (ID: %s)\n", pb.Name, pb.ID))
		sb.WriteString(fmt.Sprintf("- Score: %.2f | Confidence: %.0f%% | Version: %d | Status: %s\n",
			r.Score, pb.Confidence*100, pb.Version, pb.Status))
		if pb.Description != "" {
			sb.WriteString(fmt.Sprintf("- Description: %s\n", pb.Description))
		}
		if pb.Category != "" {
			sb.WriteString(fmt.Sprintf("- Category: %s\n", pb.Category))
		}
		if len(pb.Tags) > 0 {
			sb.WriteString(fmt.Sprintf("- Tags: %s\n", strings.Join(pb.Tags, ", ")))
		}
		sb.WriteString("- Steps:\n")
		for _, step := range pb.Steps {
			line := fmt.Sprintf("  %d. %s", step.Order, step.Action)
			if step.Tool != "" {
				line += fmt.Sprintf(" (tool: %s)", step.Tool)
			}
			if step.Expected != "" {
				line += fmt.Sprintf(" → %s", step.Expected)
			}
			sb.WriteString(line + "\n")
		}
		if len(pb.Lessons) > 0 {
			sb.WriteString("- Lessons:\n")
			for _, l := range pb.Lessons {
				sb.WriteString(fmt.Sprintf("  - %s\n", l.Content))
			}
		}
		sb.WriteString("\n")
	}

	return tools.SilentResult(sb.String())
}

// --- playbook_create ---

type PlaybookCreateTool struct {
	manager *playbookd.PlaybookManager
}

func NewPlaybookCreateTool(manager *playbookd.PlaybookManager) *PlaybookCreateTool {
	return &PlaybookCreateTool{manager: manager}
}

func (t *PlaybookCreateTool) Name() string { return "playbook_create" }

func (t *PlaybookCreateTool) Description() string {
	return "Create a new playbook with name, description, tags, category, and steps. The playbook starts in 'draft' status and can be promoted to 'active' after successful executions."
}

func (t *PlaybookCreateTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Name of the playbook",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Description of what the playbook does",
			},
			"category": map[string]any{
				"type":        "string",
				"description": "Category for organizing playbooks (e.g., 'deployment', 'debugging', 'setup')",
			},
			"tags": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Tags for searchability",
			},
			"steps": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"order":    map[string]any{"type": "integer", "description": "Step order number"},
						"action":   map[string]any{"type": "string", "description": "What to do in this step"},
						"tool":     map[string]any{"type": "string", "description": "Tool to use (optional)"},
						"expected": map[string]any{"type": "string", "description": "Expected outcome (optional)"},
						"fallback": map[string]any{"type": "string", "description": "What to do if step fails (optional)"},
						"notes":    map[string]any{"type": "string", "description": "Additional notes (optional)"},
						"optional": map[string]any{"type": "boolean", "description": "Whether this step is optional"},
					},
					"required": []string{"order", "action"},
				},
				"description": "Ordered list of steps to follow",
			},
		},
		"required": []string{"name", "description", "steps"},
	}
}

func (t *PlaybookCreateTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	name, _ := args["name"].(string)
	description, _ := args["description"].(string)
	category, _ := args["category"].(string)

	if name == "" || description == "" {
		return tools.ErrorResult("name and description are required")
	}

	// Parse tags
	var tags []string
	if rawTags, ok := args["tags"].([]any); ok {
		for _, t := range rawTags {
			if s, ok := t.(string); ok {
				tags = append(tags, s)
			}
		}
	}

	// Parse steps
	var steps []playbookd.Step
	if rawSteps, ok := args["steps"].([]any); ok {
		for _, rs := range rawSteps {
			stepMap, ok := rs.(map[string]any)
			if !ok {
				continue
			}
			step := playbookd.Step{}
			if v, ok := stepMap["order"].(float64); ok {
				step.Order = int(v)
			}
			if v, ok := stepMap["action"].(string); ok {
				step.Action = v
			}
			if v, ok := stepMap["tool"].(string); ok {
				step.Tool = v
			}
			if v, ok := stepMap["expected"].(string); ok {
				step.Expected = v
			}
			if v, ok := stepMap["fallback"].(string); ok {
				step.Fallback = v
			}
			if v, ok := stepMap["notes"].(string); ok {
				step.Notes = v
			}
			if v, ok := stepMap["optional"].(bool); ok {
				step.Optional = v
			}
			steps = append(steps, step)
		}
	}

	if len(steps) == 0 {
		return tools.ErrorResult("at least one step is required")
	}

	pb := &playbookd.Playbook{
		Name:        name,
		Description: description,
		Category:    category,
		Tags:        tags,
		Steps:       steps,
		Status:      playbookd.StatusDraft,
	}

	if err := t.manager.Create(ctx, pb); err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to create playbook: %v", err))
	}

	return tools.SilentResult(fmt.Sprintf("Playbook created successfully.\n- ID: %s\n- Name: %s\n- Slug: %s\n- Status: draft\n- Steps: %d",
		pb.ID, pb.Name, pb.Slug, len(pb.Steps)))
}

// --- playbook_record ---

type PlaybookRecordTool struct {
	manager *playbookd.PlaybookManager
	agentID string
}

func NewPlaybookRecordTool(manager *playbookd.PlaybookManager, agentID string) *PlaybookRecordTool {
	return &PlaybookRecordTool{manager: manager, agentID: agentID}
}

func (t *PlaybookRecordTool) Name() string { return "playbook_record" }

func (t *PlaybookRecordTool) Description() string {
	return "Record the execution of a playbook with outcome (success/partial/failure), step results, and reflection. Updates the playbook's confidence score via Wilson score interval."
}

func (t *PlaybookRecordTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"playbook_id": map[string]any{
				"type":        "string",
				"description": "ID of the playbook that was executed",
			},
			"outcome": map[string]any{
				"type":        "string",
				"enum":        []string{"success", "partial", "failure"},
				"description": "Overall execution outcome",
			},
			"task_context": map[string]any{
				"type":        "string",
				"description": "Description of the task context in which the playbook was executed",
			},
			"step_results": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"step_order": map[string]any{"type": "integer"},
						"outcome":    map[string]any{"type": "string", "enum": []string{"success", "partial", "failure"}},
						"output":     map[string]any{"type": "string", "description": "Output or result of the step"},
						"error":      map[string]any{"type": "string", "description": "Error message if step failed"},
						"duration":   map[string]any{"type": "string", "description": "Duration of the step (e.g., '2s', '1m30s')"},
					},
					"required": []string{"step_order", "outcome"},
				},
				"description": "Results for each step executed",
			},
			"reflection": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"what_worked":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"what_failed":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"improvements":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"should_update": map[string]any{"type": "boolean", "description": "Whether the playbook should be updated based on this reflection"},
				},
				"description": "Reflection on what worked, failed, and improvements",
			},
		},
		"required": []string{"playbook_id", "outcome"},
	}
}

func (t *PlaybookRecordTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	playbookID, _ := args["playbook_id"].(string)
	outcomeStr, _ := args["outcome"].(string)
	taskContext, _ := args["task_context"].(string)

	if playbookID == "" || outcomeStr == "" {
		return tools.ErrorResult("playbook_id and outcome are required")
	}

	outcome := playbookd.Outcome(outcomeStr)

	// Verify playbook exists
	pb, err := t.manager.Get(ctx, playbookID)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("playbook not found: %v", err))
	}

	// Parse step results
	var stepResults []playbookd.StepResult
	if rawSteps, ok := args["step_results"].([]any); ok {
		for _, rs := range rawSteps {
			stepMap, ok := rs.(map[string]any)
			if !ok {
				continue
			}
			sr := playbookd.StepResult{}
			if v, ok := stepMap["step_order"].(float64); ok {
				sr.StepOrder = int(v)
			}
			if v, ok := stepMap["outcome"].(string); ok {
				sr.Outcome = playbookd.Outcome(v)
			}
			if v, ok := stepMap["output"].(string); ok {
				sr.Output = v
			}
			if v, ok := stepMap["error"].(string); ok {
				sr.Error = v
			}
			if v, ok := stepMap["duration"].(string); ok {
				sr.Duration = v
			}
			stepResults = append(stepResults, sr)
		}
	}

	// Parse reflection
	var reflection *playbookd.Reflection
	if rawRef, ok := args["reflection"].(map[string]any); ok {
		reflection = &playbookd.Reflection{}
		if v, ok := rawRef["what_worked"].([]any); ok {
			for _, item := range v {
				if s, ok := item.(string); ok {
					reflection.WhatWorked = append(reflection.WhatWorked, s)
				}
			}
		}
		if v, ok := rawRef["what_failed"].([]any); ok {
			for _, item := range v {
				if s, ok := item.(string); ok {
					reflection.WhatFailed = append(reflection.WhatFailed, s)
				}
			}
		}
		if v, ok := rawRef["improvements"].([]any); ok {
			for _, item := range v {
				if s, ok := item.(string); ok {
					reflection.Improvements = append(reflection.Improvements, s)
				}
			}
		}
		if v, ok := rawRef["should_update"].(bool); ok {
			reflection.ShouldUpdate = v
		}
	}

	now := time.Now()
	record := &playbookd.ExecutionRecord{
		PlaybookID:  playbookID,
		PlaybookVer: pb.Version,
		AgentID:     t.agentID,
		StartedAt:   now,
		CompletedAt: now,
		Outcome:     outcome,
		StepResults: stepResults,
		TaskContext: taskContext,
		Reflection:  reflection,
	}

	if err := t.manager.RecordExecution(ctx, record); err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to record execution: %v", err))
	}

	// Apply reflection if provided and should_update is true
	if reflection != nil && reflection.ShouldUpdate {
		if err := t.manager.ApplyReflection(ctx, playbookID, reflection); err != nil {
			return tools.SilentResult(fmt.Sprintf("Execution recorded (ID: %s), but failed to apply reflection: %v", record.ID, err))
		}
	}

	// Fetch updated playbook for confidence info
	updatedPb, err := t.manager.Get(ctx, playbookID)
	if err != nil {
		return tools.SilentResult(fmt.Sprintf("Execution recorded (ID: %s), outcome: %s", record.ID, outcome))
	}

	result, _ := json.Marshal(map[string]any{
		"execution_id": record.ID,
		"outcome":      outcome,
		"confidence":   fmt.Sprintf("%.0f%%", updatedPb.Confidence*100),
		"success_rate": fmt.Sprintf("%.0f%%", updatedPb.SuccessRate*100),
		"version":      updatedPb.Version,
		"status":       updatedPb.Status,
	})

	return tools.SilentResult(fmt.Sprintf("Execution recorded successfully.\n%s", string(result)))
}

// --- playbook_list ---

type PlaybookListTool struct {
	manager *playbookd.PlaybookManager
}

func NewPlaybookListTool(manager *playbookd.PlaybookManager) *PlaybookListTool {
	return &PlaybookListTool{manager: manager}
}

func (t *PlaybookListTool) Name() string { return "playbook_list" }

func (t *PlaybookListTool) Description() string {
	return "List playbooks with optional filters by category and status."
}

func (t *PlaybookListTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"category": map[string]any{
				"type":        "string",
				"description": "Filter by category (optional)",
			},
			"status": map[string]any{
				"type":        "string",
				"enum":        []string{"draft", "active", "deprecated", "archived"},
				"description": "Filter by status (optional)",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Maximum number of results (default: 20)",
			},
		},
	}
}

func (t *PlaybookListTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	filter := playbookd.ListFilter{
		Limit: 20,
	}

	if category, ok := args["category"].(string); ok && category != "" {
		filter.Category = category
	}
	if statusStr, ok := args["status"].(string); ok && statusStr != "" {
		status := playbookd.Status(statusStr)
		filter.Status = &status
	}
	if l, ok := args["limit"].(float64); ok && l > 0 {
		filter.Limit = int(l)
	}

	playbooks, err := t.manager.List(ctx, filter)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to list playbooks: %v", err))
	}

	if len(playbooks) == 0 {
		return tools.SilentResult("No playbooks found.")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d playbook(s):\n\n", len(playbooks)))
	for _, pb := range playbooks {
		sb.WriteString(fmt.Sprintf("- **%s** (ID: %s)\n", pb.Name, pb.ID))
		sb.WriteString(fmt.Sprintf("  Status: %s | Confidence: %.0f%% | Version: %d | Steps: %d\n",
			pb.Status, pb.Confidence*100, pb.Version, len(pb.Steps)))
		if pb.Category != "" {
			sb.WriteString(fmt.Sprintf("  Category: %s\n", pb.Category))
		}
		if pb.Description != "" {
			desc := pb.Description
			if len(desc) > 100 {
				desc = desc[:100] + "..."
			}
			sb.WriteString(fmt.Sprintf("  Description: %s\n", desc))
		}
	}

	return tools.SilentResult(sb.String())
}
