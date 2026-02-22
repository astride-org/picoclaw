package tasks

const TaskFinishedMarker = "[TASK-FINISHED]"

// TaskInfo is the result of task detection by a channel.
type TaskInfo struct {
	IsTask      bool
	Description string
	TraceID     string // channel-agnostic: thread ID (Discord), thread_ts (Slack), etc.
}

// TaskDetector is the interface that each channel implements to detect tasks.
// Each channel decides its own detection and completion logic.
type TaskDetector interface {
	// Detect checks if a message belongs to a task context.
	Detect(channelID string) (TaskInfo, bool)

	// IsFinished checks if a message signals the end of a task.
	IsFinished(content string, metadata map[string]string) bool
}

// TaskMetadata holds parsed task metadata from message metadata maps.
type TaskMetadata struct {
	TaskMode        bool
	TaskDescription string
	TaskFinished    bool
	TraceID         string
}

// ParseMetadata extracts task metadata from a generic metadata map.
func ParseMetadata(metadata map[string]string) TaskMetadata {
	return TaskMetadata{
		TaskMode:        metadata["task_mode"] == "true",
		TaskDescription: metadata["task_description"],
		TaskFinished:    metadata["task_finished"] == "true",
		TraceID:         metadata["trace_id"],
	}
}

// ApplyTaskMetadata sets task-related keys in a metadata map.
func ApplyTaskMetadata(metadata map[string]string, info TaskInfo) {
	metadata["task_mode"] = "true"
	metadata["task_description"] = info.Description
	metadata["trace_id"] = info.TraceID
}
