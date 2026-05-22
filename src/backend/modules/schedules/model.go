// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package schedules

// Schedule represents a scheduled task.
type Schedule struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Cron        string `json:"cron"` // cron expression
	Action      string `json:"action"`
	Target      string `json:"target"` // container ID or name
	EnvID       string `json:"envId"`
	Enabled     bool   `json:"enabled"`
	LastRunAt   string `json:"lastRunAt"`
	NextRunAt   string `json:"nextRunAt"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// Execution represents a single schedule execution record.
type Execution struct {
	ID         string `json:"id"`
	ScheduleID string `json:"scheduleId"`
	Status     string `json:"status"` // success, failed
	Output     string `json:"output"`
	Duration   int    `json:"duration"` // milliseconds
	ExecutedAt string `json:"executedAt"`
}

// CreateScheduleInput is the request body for creating a schedule.
type CreateScheduleInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Cron        string `json:"cron"`
	Action      string `json:"action"` // start, stop, restart, exec
	Target      string `json:"target"`
	EnvID       string `json:"envId"`
}

// UpdateScheduleInput is the request body for updating a schedule.
type UpdateScheduleInput struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Cron        *string `json:"cron"`
	Action      *string `json:"action"`
	Target      *string `json:"target"`
	EnvID       *string `json:"envId"`
	Enabled     *bool   `json:"enabled"`
}
