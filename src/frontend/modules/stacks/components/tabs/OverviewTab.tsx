// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconClearAll } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Card, CardContent, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { StatusBadge, STACK_STATUS } from '@resources/components/ui/StatusBadge';
import { formatDate } from '@resources/utils/format';
import type { StackDetail } from '../../hooks/useStacks';
import { usePruneStack } from '../../hooks/useStacks';

type OverviewTabProps = {
  stack: StackDetail;
  editing?: boolean;
  description?: string;
  onDescriptionChange?: (value: string) => void;
};

export function OverviewTab({ stack, editing, description, onDescriptionChange }: OverviewTabProps) {
  const { t } = useTranslation('stacks');
  const running = stack.services.filter((svc) => svc.status === 'running').length;
  const pruneStack = usePruneStack();
  const [pruneResult, setPruneResult] = useState<{ removed: string[]; count: number } | null>(null);

  const handlePrune = () => {
    pruneStack.mutate(stack.name, {
      onSuccess: (result) => setPruneResult(result),
    });
  };

  return (
    <div className="grid gap-6 lg:grid-cols-2">
      {/* Stack info card */}
      <Card>
        <CardHeader>
          <CardTitle className="text-sm">{t('detail.stackInfo')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <InfoRow label={t('detail.name')} value={stack.name} />
          <InfoRow
            label={t('detail.status')}
            value={<StatusBadge status={stack.status} map={STACK_STATUS} />}
          />
          <InfoRow
            label={t('detail.type')}
            value={
              <Badge
                variant={stack.type === 'managed' ? 'default' : 'outline'}
                className="text-[10px] px-1.5 py-0"
              >
                {stack.type === 'managed' ? t('badges.managed') : t('badges.discovered')}
              </Badge>
            }
          />
          {editing ? (
            <div className="space-y-1">
              <span className="text-sm text-muted-foreground">{t('detail.description')}</span>
              <textarea
                value={description ?? ''}
                onChange={(e) => onDescriptionChange?.(e.target.value)}
                placeholder={t('detail.descriptionPlaceholder')}
                rows={3}
                className="w-full rounded-md border border-border bg-muted px-3 py-2 text-sm text-foreground focus:border-primary focus:outline-none resize-none"
              />
            </div>
          ) : (
            stack.description && (
              <InfoRow label={t('detail.description')} value={stack.description} />
            )
          )}
          <InfoRow label={t('detail.created')} value={formatDate(stack.createdAt)} />
          <InfoRow label={t('detail.updated')} value={formatDate(stack.updatedAt)} />
        </CardContent>
      </Card>

      {/* Services summary card */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle className="text-sm">
              {t('detail.servicesCount')}
              <span className="ml-2 text-xs font-normal text-muted-foreground">
                {t('detail.runningServices', { running, total: stack.services.length })}
              </span>
            </CardTitle>
            {stack.type === 'managed' && (
              <Button
                variant="outline"
                size="sm"
                onClick={handlePrune}
                disabled={pruneStack.isPending}
              >
                <IconClearAll className="mr-1 size-3.5" />
                {pruneStack.isPending ? t('prune.pruning') : t('prune.button')}
              </Button>
            )}
          </div>
        </CardHeader>
        <CardContent>
          {pruneResult && (
            <div className="mb-3 rounded-md border border-border bg-muted/50 px-3 py-2 text-xs text-muted-foreground">
              {pruneResult.count === 0
                ? t('prune.noOrphans')
                : t('prune.removed', { count: pruneResult.count })}
              {pruneResult.removed.length > 0 && (
                <ul className="mt-1 list-disc pl-4">
                  {pruneResult.removed.map((name) => (
                    <li key={name}>{name}</li>
                  ))}
                </ul>
              )}
            </div>
          )}
          <div className="space-y-2">
            {stack.services.map((svc) => (
              <div
                key={svc.name}
                className="flex items-center justify-between rounded-lg border border-border px-3 py-2"
              >
                <div className="flex items-center gap-3">
                  <span className="text-sm font-medium">{svc.name}</span>
                  <span className="text-xs text-muted-foreground">
                    {svc.image.split(':')[0]?.split('/').pop()}
                  </span>
                </div>
                <Badge
                  variant={svc.status === 'running' ? 'success' : 'destructive'}
                  className="text-[10px] px-1.5 py-0"
                >
                  {svc.status}
                </Badge>
              </div>
            ))}
            {stack.services.length === 0 && (
              <p className="py-4 text-center text-sm text-muted-foreground">
                {t('detail.noServices')}
              </p>
            )}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

function InfoRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between text-sm">
      <span className="text-muted-foreground">{label}</span>
      <span className="font-medium">{value}</span>
    </div>
  );
}
