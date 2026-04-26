package models

import "time"


const (
	TempDir = "/tmp"
)

type ImportOptions struct {
	RepoURL string `json:"repo_url"`
	AuthToken string `json:"auth_token"`
	
}


type DeploymentStatus string

const (
	StatusPending    DeploymentStatus = "pending"
	StatusBuilding   DeploymentStatus = "building"
	StatusDeploying  DeploymentStatus = "deploying"
	StatusRunning    DeploymentStatus = "running"
	StatusFailed     DeploymentStatus = "failed"
	StatusRolledBack DeploymentStatus = "rolled_back"
	StatusDestroyed  DeploymentStatus = "destroyed"
)

type SourceType string

const (
	SourceGit    SourceType = "git"
	SourceUpload SourceType = "upload"
)

type Deployment struct {
	ID                string           `json:"id"`
	Name              string           `json:"name"`
	SourceType        SourceType       `json:"source_type"`
	ResourceURL            string           `json:"resource_url,omitempty"`
	GitBranch         string           `json:"git_branch,omitempty"`
	GitCommit         string           `json:"git_commit,omitempty"`
	EnvVars           map[string]string `json:"env_vars,omitempty"`
	Status            DeploymentStatus `json:"status"`
	ImageTag          string           `json:"image_tag,omitempty"`
	ActiveContainerID string           `json:"active_container_id,omitempty"`
	StandbyContainerID string          `json:"standby_container_id,omitempty"`
	CaddyRoute        string           `json:"caddy_route,omitempty"`
	Port              int              `json:"port,omitempty"`
	CreatedAt         time.Time        `json:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at"`
}



type LogLine struct {
	ID           int64     `json:"id"`
	DeploymentID string    `json:"deployment_id"`
	Stream       string    `json:"stream"`
	Text         string    `json:"text"`
	Sequence     int64     `json:"sequence"`
	CreatedAt    time.Time `json:"created_at"`
}
