// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { GitOpsPipeline, GitOpsStage } from '../types';

export function PipelineStages({ pipeline, t }: { pipeline: GitOpsPipeline; t: (key: string) => string }) {
  return (
    <div>
      <h3 className="mb-2 text-sm font-semibold text-foreground">{t('formStages')}</h3>
      <div className="space-y-2">
        {pipeline.stages.map((s: GitOpsStage, i: number) => (
          <div key={s.id} className="mc-glass rounded-md px-3 py-2 text-sm">
            <div className="flex items-center justify-between">
              <div className="font-medium">
                {i + 1}. {s.stageName}
              </div>
              <div className="text-xs text-muted-foreground">
                {s.branch} · {s.deployMode}
                {s.requiresApproval && ` · ${t('approvalRequiredSuffix')}`}
              </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
