// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package gitops

import "context"

// ValidateStages applies the same checks CreatePipeline and UpdatePipeline make,
// so rows written outside the HTTP path — the demo seed — are held to them too.
func (s *Service) ValidateStages(ctx context.Context, userID string, stages []StageInput) error {
	return s.validateStages(ctx, userID, stages)
}

// StageStackName is the stack a stage's promotions deploy to; a seeder that
// writes a different name would produce a stack the next promotion cannot adopt.
func StageStackName(pipeline *Pipeline, stage *Stage) string {
	return stageStackName(pipeline, stage)
}

// StageStackOwner is the managed_by provenance a stage's stack carries, which the
// stacks module checks before letting a promotion update or remove it.
func StageStackOwner(pipeline *Pipeline, stage *Stage) string {
	return stageStackOwner(pipeline, stage)
}

// PreviewStackName is the stack a preview deploys inside its own environment.
func PreviewStackName(pipeline *Pipeline, preview *PRPreview) string {
	return previewStackName(pipeline, preview)
}

// PreviewStackOwner is the managed_by provenance a preview's stack carries.
func PreviewStackOwner(previewID string) string {
	return previewStackOwner(previewID)
}

// PreviewEnvironmentName is the display name a preview's generated environment
// gets, naming the pipeline and the pull request that created it.
func PreviewEnvironmentName(pipeline *Pipeline, preview *PRPreview) string {
	return previewEnvironmentName(pipeline, preview)
}

// TriggerKindFor is the trigger kind a promotion is recorded with, inferred from
// who triggered it the same way beginPromotion does.
func TriggerKindFor(triggeredBy, prNumber string) TriggerKind {
	return inferredTriggerKind(triggeredBy, prNumber)
}

// PreviewURL resolves the pipeline's hostname template for a preview, or returns
// empty when the pipeline configures no preview domain.
func PreviewURL(pipeline *Pipeline, preview *PRPreview) string {
	return previewURL(pipeline, preview)
}
