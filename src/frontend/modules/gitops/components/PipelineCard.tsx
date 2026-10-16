// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconRocket, IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { PlainButton } from '@resources/components/ui/PlainButton';
import { PERM } from '@core/auth/permissionKeys';
import type { GitOpsPipeline } from '../types';

export function PipelineCard({
  pipeline,
  selected,
  onSelect,
  onEdit,
  onPromote,
  onDelete,
}: {
  pipeline: GitOpsPipeline;
  selected: boolean;
  onSelect: () => void;
  onEdit: () => void;
  onPromote: () => void;
  onDelete: () => void;
}) {
  const { t } = useTranslation('gitops');
  const triggerLabel =
    pipeline.triggerType === 'auto'
      ? t('triggerAuto')
      : pipeline.triggerType === 'manual'
        ? t('triggerManual')
        : t('triggerPRPreview');

  return (
    <div
      className={`relative rounded-lg border p-3 transition-colors ${
        selected ? 'border-primary bg-primary/5' : 'border-border bg-card hover:border-primary/40'
      }`}
    >
      <PlainButton
        aria-label={pipeline.name}
        aria-pressed={selected}
        className="absolute inset-0 cursor-pointer rounded-lg"
        onClick={onSelect}
      />
      <div className="pointer-events-none relative flex items-start justify-between">
        <div className="min-w-0 flex-1">
          <div className="truncate font-medium text-foreground">{pipeline.name}</div>
          <div className="text-xs text-muted-foreground">
            {pipeline.stages.length} {t('columnStages').toLowerCase()} · {triggerLabel}
          </div>
        </div>
      </div>
      <div className="relative mt-2 flex gap-1">
        <Button
          permission={PERM.GITOPS_MANAGE}
          size="sm"
          variant="outline"
          onClick={(e) => {
            e.stopPropagation();
            onPromote();
          }}
        >
          <IconRocket className="mr-1.5 size-3.5" />
          {t('promoteAction')}
        </Button>
        <Button
          permission={PERM.GITOPS_MANAGE}
          size="sm"
          variant="ghost"
          onClick={(e) => {
            e.stopPropagation();
            onEdit();
          }}
        >
          {t('formActions.update')}
        </Button>
        <Button
          permission={PERM.GITOPS_DELETE}
          size="icon"
          variant="ghost"
          aria-label={t('formActions.delete')}
          onClick={(e) => {
            e.stopPropagation();
            onDelete();
          }}
        >
          <IconTrash className="size-4" />
        </Button>
      </div>
    </div>
  );
}
