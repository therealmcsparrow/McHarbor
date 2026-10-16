// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package bootstrap

import (
	"context"
	"fmt"

	ai_tools "github.com/therealmcsparrow/mcharbor/modules/ai_assistant/tools"
	"github.com/therealmcsparrow/mcharbor/modules/alerts"
	"github.com/therealmcsparrow/mcharbor/modules/schedules"
	"github.com/therealmcsparrow/mcharbor/modules/updates"
	"github.com/therealmcsparrow/mcharbor/modules/workflows"
)

// alertCatalogAdapter hands the alert condition catalog to the AI tools without
// letting that package import the alerts module.
type alertCatalogAdapter struct{}

func (a *alertCatalogAdapter) AlertConditions(ctx context.Context) (any, error) {
	return alerts.Catalog(), nil
}

// workflowAuthorAdapter creates through Import rather than Create, because only
// Import validates the canvas JSON and pins the new workflow to draft status.
type workflowAuthorAdapter struct {
	svc *workflows.Service
}

func (a *workflowAuthorAdapter) CreateWorkflow(ctx context.Context, in ai_tools.WorkflowDraft) (map[string]any, error) {
	if a.svc == nil {
		return nil, fmt.Errorf("workflows service not wired")
	}
	created, err := a.svc.Import(ctx, workflows.WorkflowExport{
		Kind:    "mcharbor.workflow",
		Version: 1,
		Workflow: workflows.WorkflowExportRecord{
			Name:        in.Name,
			Description: in.Description,
			CanvasData:  in.CanvasData,
			Variables:   in.Variables,
		},
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"id":          created.ID,
		"name":        created.Name,
		"description": created.Description,
		"status":      created.Status,
		"createdAt":   created.CreatedAt,
	}, nil
}

// alertAuthorAdapter flattens the tool's draft onto the alerts module's nested
// create input, where the condition registry validates the source/condition pair.
type alertAuthorAdapter struct {
	svc *alerts.Service
}

func (a *alertAuthorAdapter) CreateAlert(ctx context.Context, in ai_tools.AlertDraft) (map[string]any, error) {
	if a.svc == nil {
		return nil, fmt.Errorf("alerts service not wired")
	}
	destinations := make([]alerts.DestinationInput, 0, len(in.DestinationChannelIDs))
	for _, channelID := range in.DestinationChannelIDs {
		destinations = append(destinations, alerts.DestinationInput{ChannelID: channelID})
	}
	notifyOnRecovery := in.NotifyOnRecovery
	created, err := a.svc.Create(ctx, alerts.CreateAlertInput{
		Name:         in.Name,
		Severity:     in.Severity,
		SourceType:   in.SourceType,
		ConditionKey: in.ConditionKey,
		Condition: alerts.ConditionConfig{
			Operator:      in.Operator,
			Threshold:     in.Threshold,
			Unit:          in.Unit,
			ForSeconds:    in.ForSeconds,
			WindowSeconds: in.WindowSeconds,
		},
		Target:           in.Target,
		EnvironmentIDs:   in.EnvironmentIDs,
		Destinations:     destinations,
		SendInApp:        in.SendInApp,
		CooldownSeconds:  in.CooldownSeconds,
		NotifyOnRecovery: &notifyOnRecovery,
		MessageTemplate:  in.MessageTemplate,
	})
	if err != nil {
		return nil, err
	}
	if created == nil {
		return nil, fmt.Errorf("alert was not created")
	}
	return map[string]any{
		"id":           created.ID,
		"name":         created.Name,
		"severity":     created.Severity,
		"sourceType":   created.SourceType,
		"conditionKey": created.ConditionKey,
		"target":       created.Target,
		"enabled":      created.Enabled,
		"createdAt":    created.CreatedAt,
	}, nil
}

// updatePolicyAuthorAdapter creates an update policy on behalf of the AI action plane.
type updatePolicyAuthorAdapter struct {
	svc *updates.Service
}

func (a *updatePolicyAuthorAdapter) CreateUpdatePolicy(ctx context.Context, in ai_tools.UpdatePolicyDraft) (map[string]any, error) {
	if a.svc == nil {
		return nil, fmt.Errorf("updates service not wired")
	}
	created, err := a.svc.Create(ctx, updates.CreatePolicyInput{
		Name:           in.Name,
		ContainerMatch: in.ContainerMatch,
		ImageMatch:     in.ImageMatch,
		Schedule:       in.Schedule,
		Strategy:       in.Strategy,
		AutoRestart:    in.AutoRestart,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"id":             created.ID,
		"name":           created.Name,
		"containerMatch": created.ContainerMatch,
		"imageMatch":     created.ImageMatch,
		"schedule":       created.Schedule,
		"strategy":       created.Strategy,
		"autoRestart":    created.AutoRestart,
		"enabled":        created.Enabled,
		"createdAt":      created.CreatedAt,
	}, nil
}

// scheduleAuthorAdapter creates a scheduled automation on behalf of the AI action plane.
type scheduleAuthorAdapter struct {
	svc *schedules.Service
}

func (a *scheduleAuthorAdapter) CreateSchedule(ctx context.Context, in ai_tools.ScheduleDraft) (map[string]any, error) {
	if a.svc == nil {
		return nil, fmt.Errorf("schedules service not wired")
	}
	created, err := a.svc.Create(ctx, schedules.CreateScheduleInput{
		Name:        in.Name,
		Description: in.Description,
		Cron:        in.Cron,
		Action:      in.Action,
		Target:      in.Target,
		EnvID:       in.EnvID,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"id":          created.ID,
		"name":        created.Name,
		"cron":        created.Cron,
		"action":      created.Action,
		"target":      created.Target,
		"envId":       created.EnvID,
		"enabled":     created.Enabled,
		"createdAt":   created.CreatedAt,
		"description": created.Description,
	}, nil
}
