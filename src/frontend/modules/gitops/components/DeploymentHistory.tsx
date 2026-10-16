// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Spinner } from '@resources/components/ui/Spinner';
import { StatusError } from '@resources/components/ui/StatusError';
import type { GitOpsPromotion } from '../types';

export function DeploymentHistory({
  promotions,
  isLoading,
  t,
}: {
  promotions: GitOpsPromotion[];
  isLoading: boolean;
  t: (key: string) => string;
}) {
  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-4">
        <Spinner />
      </div>
    );
  }
  if (promotions.length === 0) {
    return (
      <div>
        <h3 className="mb-2 text-sm font-semibold text-foreground">{t('deployments')}</h3>
        <p className="text-sm text-muted-foreground">{t('noDeployments')}</p>
      </div>
    );
  }
  return (
    <div>
      <h3 className="mb-2 text-sm font-semibold text-foreground">{t('deployments')}</h3>
      <div className="space-y-1">
        {promotions.map((p) => (
          <div key={p.id} className="mc-glass rounded-md px-3 py-2 text-sm">
            <div className="flex items-center justify-between">
              <code className="text-xs">{p.commitSha.slice(0, 8)}</code>
              <div className="flex items-center gap-2">
                <span className="text-xs text-muted-foreground">{p.status}</span>
                <StatusError error={p.lastError} />
              </div>
            </div>
            <div className="text-xs text-muted-foreground">
              {p.triggerKind} · {p.startedAt}
              {p.stackName && (
                <>
                  {' '}
                  · <code>{p.stackName}</code>
                </>
              )}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
