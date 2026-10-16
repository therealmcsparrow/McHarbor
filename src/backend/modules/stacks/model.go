// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package stacks

import "github.com/therealmcsparrow/mcharbor/core/selfguard"

// Stack represents a Compose stack (discovered from Docker or stored in the DB).
// Orchestrator values for Stack.Orchestrator. Swarm stacks are owned by the
// swarm add-on plugin, which carries its own orchestrator constant.
const (
	OrchestratorCompose = "compose"
)

// Stack is a Compose project, either managed by McHarbor from the database or discovered from container labels.
type Stack struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	EnvironmentID *string    `json:"environmentId,omitempty"`
	ProjectPath   string     `json:"projectPath,omitempty"`
	ComposeFile   string     `json:"composeFile,omitempty"`
	Status        string     `json:"status"`
	Description   *string    `json:"description,omitempty"`
	Services      []StackSvc `json:"services"`
	OwnerUserID   *string    `json:"ownerUserId,omitempty"` // Phase 4 of the RBAC roadmap
	// OwnerName is the owning user's username, joined in listFromDB only —
	// a managed stack with no owner_user_id, or a discovered stack (which
	// never has one), leaves this nil.
	OwnerName *string `json:"ownerName,omitempty"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
	Type      string  `json:"type"` // "managed" (DB) or "discovered" (Docker labels)
	// SecretFileMode is this stack's file-injection setting. See
	// modules/stacks/filemode.go.
	SecretFileMode bool `json:"secretFileMode"`
	// Orchestrator is "compose" or "swarm". It is orthogonal to Type: a
	// stack may be managed or discovered under either orchestrator.
	Orchestrator string `json:"orchestrator"`
	Protected    bool   `json:"protected"`
	TargetsSelf  bool   `json:"targetsSelf"`
}

// StackSvc represents a running service within a Compose stack.
type StackSvc struct {
	Name        string `json:"name"`
	ContainerID string `json:"containerId,omitempty"`
	Status      string `json:"status"`
	Image       string `json:"image"`
}

// CreateRequest is the JSON body for POST /stacks.
type CreateRequest struct {
	Name          string            `json:"name"`
	Compose       string            `json:"compose"`
	EnvVars       map[string]string `json:"envVars,omitempty"`
	Description   *string           `json:"description,omitempty"`
	EnvironmentID *string           `json:"environmentId,omitempty"`
	AutoStart     bool              `json:"autoStart"`
	// SecretFileMode delivers every ${secret:NAME} this stack references as a
	// file under /run/secrets and sets <VAR>_FILE to its path, instead of
	// putting the resolved value in the containers' environment.
	SecretFileMode bool `json:"secretFileMode,omitempty"`
}

// UpdateRequest is the JSON body for PUT /stacks/{name}.
type UpdateRequest struct {
	Compose     *string           `json:"compose,omitempty"`
	Description *string           `json:"description,omitempty"`
	EnvVars     map[string]string `json:"envVars,omitempty"`
	Name        *string           `json:"name,omitempty"`
	// Nil leaves the stack's current setting alone; the flag is a property of
	// the stack, not of one edit.
	SecretFileMode *bool `json:"secretFileMode,omitempty"`
}

// ComposeResult holds stdout/stderr from a docker compose command.
type ComposeResult struct {
	Success bool   `json:"success"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
	// Refusal is set when the host-access check refused the up before compose ran, so the
	// handler can answer 403 with the key instead of a generic deploy failure.
	Refusal *selfguard.Refusal `json:"-"`
}

// AdoptPreviewRequest is the JSON body for POST /stacks/adopt/preview.
type AdoptPreviewRequest struct {
	StackName   string `json:"stackName"`
	ContainerID string `json:"containerId,omitempty"`
}

// AdoptPreviewResponse contains the generated compose preview.
type AdoptPreviewResponse struct {
	Name    string `json:"name"`
	Compose string `json:"compose"`
}

// AdoptRequest is the JSON body for POST /stacks/adopt.
type AdoptRequest struct {
	Name        string `json:"name"`
	Compose     string `json:"compose"`
	Description string `json:"description,omitempty"`
	ContainerID string `json:"containerId,omitempty"`
}

// ContainerStackLink represents a manual container-to-stack relationship.
type ContainerStackLink struct {
	ID            string `json:"id"`
	EnvironmentID string `json:"environmentId"`
	ContainerID   string `json:"containerId"`
	StackName     string `json:"stackName"`
	ServiceName   string `json:"serviceName,omitempty"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

// LinkContainerRequest is the JSON body for linking a container to a stack.
type LinkContainerRequest struct {
	ContainerID string `json:"containerId"`
	StackName   string `json:"stackName"`
	ServiceName string `json:"serviceName,omitempty"`
}

// StackWebhook represents a webhook that fires on stack events.
type StackWebhook struct {
	ID        string `json:"id"`
	StackID   string `json:"stackId"`
	URL       string `json:"url"`
	Secret    string `json:"secret,omitempty"`
	Events    string `json:"events"`
	IsActive  bool   `json:"isActive"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// CreateStackWebhookInput is the JSON body for creating a webhook.
type CreateStackWebhookInput struct {
	URL    string `json:"url"`
	Secret string `json:"secret,omitempty"`
	Events string `json:"events"`
}

// UpdateStackWebhookInput is the JSON body for updating a webhook.
type UpdateStackWebhookInput struct {
	URL      *string `json:"url,omitempty"`
	Secret   *string `json:"secret,omitempty"`
	Events   *string `json:"events,omitempty"`
	IsActive *bool   `json:"isActive,omitempty"`
}

// PruneResult holds the result of an orphan prune operation.
type PruneResult struct {
	Removed []string `json:"removed"`
	Count   int      `json:"count"`
}

// ServiceUpdateResult holds the update-check result for a single service image.
type ServiceUpdateResult struct {
	ServiceName     string `json:"serviceName"`
	ContainerID     string `json:"containerId,omitempty"`
	Image           string `json:"image"`
	CurrentDigest   string `json:"currentDigest"`
	RemoteDigest    string `json:"remoteDigest"`
	UpdateAvailable bool   `json:"updateAvailable"`
	Error           string `json:"error,omitempty"`
}

// StackUpdateResult holds update-check results for all services in a stack.
type StackUpdateResult struct {
	StackName       string                `json:"stackName"`
	UpdateAvailable bool                  `json:"updateAvailable"`
	Services        []ServiceUpdateResult `json:"services"`
}
