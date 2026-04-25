package models
type LogEntry struct {
	DeploymentID string
	Stream       string // stdout | stderr | system
	Text         string
}

