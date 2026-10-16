// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { PlainButton } from '@resources/components/ui/PlainButton';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { IconArrowUpCircle, IconBoxOff, IconFileText, IconPhotoOff, IconTrash } from '@tabler/icons-react';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { Spinner } from '@resources/components/ui/Spinner';
import { cn } from '@resources/utils/cn';
import { usePruneImages } from '@resources/hooks/useImages';
import { usePruneVolumes } from '@modules/volumes/hooks/useVolumes';
import { usePruneContainers } from '@modules/containers/hooks/useContainers';
import { useCheckContainerUpdates } from '@modules/containers/hooks/useContainerUpdates';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';
import { TONE_SOFT, WidgetFrame, type Tone } from '@modules/dashboard/components/widget-kit';

type ActionKey = 'pruneImages' | 'pruneVolumes' | 'pruneContainers' | 'checkUpdates' | 'liveLogs';

type Action = {
  key: ActionKey;
  icon: typeof IconTrash;
  tone: Tone;
  label: string;
  hint: string;
  /** Destructive actions confirm first; the rest run immediately. */
  confirm?: string;
  run: () => void;
  isPending: boolean;
};

export default function QuickActionsWidget({ typeId: _typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const navigate = useNavigate();
  const [confirmAction, setConfirmAction] = useState<ActionKey | null>(null);

  const pruneImages = usePruneImages();
  const pruneVolumes = usePruneVolumes();
  const pruneContainers = usePruneContainers();
  const checkUpdates = useCheckContainerUpdates();
  const dismiss = () => setConfirmAction(null);

  const actions: Action[] = [
    {
      key: 'checkUpdates',
      icon: IconArrowUpCircle,
      tone: 'info',
      label: t('quickActionsWidget.checkUpdates'),
      hint: t('quickActionsWidget.checkUpdatesDesc'),
      run: () => checkUpdates.mutate(undefined),
      isPending: checkUpdates.isPending,
    },
    {
      key: 'liveLogs',
      icon: IconFileText,
      tone: 'primary',
      label: t('quickActionsWidget.liveLogs'),
      hint: t('quickActionsWidget.liveLogsDesc'),
      run: () => navigate('/logs'),
      isPending: false,
    },
    {
      key: 'pruneContainers',
      icon: IconBoxOff,
      tone: 'warning',
      label: t('quickActionsWidget.pruneContainers'),
      hint: t('quickActionsWidget.pruneContainersDesc'),
      confirm: t('quickActionsWidget.pruneContainersDesc'),
      run: () => pruneContainers.mutate(undefined, { onSettled: dismiss }),
      isPending: pruneContainers.isPending,
    },
    {
      key: 'pruneImages',
      icon: IconPhotoOff,
      tone: 'warning',
      label: t('quickActionsWidget.pruneImages'),
      hint: t('quickActionsWidget.pruneImagesDesc'),
      confirm: t('quickActionsWidget.pruneImagesDesc'),
      run: () => pruneImages.mutate(undefined, { onSettled: dismiss }),
      isPending: pruneImages.isPending,
    },
    {
      key: 'pruneVolumes',
      icon: IconTrash,
      tone: 'danger',
      label: t('quickActionsWidget.pruneVolumes'),
      hint: t('quickActionsWidget.pruneVolumesDesc'),
      confirm: t('quickActionsWidget.pruneVolumesDesc'),
      run: () => pruneVolumes.mutate(undefined, { onSettled: dismiss }),
      isPending: pruneVolumes.isPending,
    },
  ];

  const active = actions.find((a) => a.key === confirmAction);

  return (
    <WidgetFrame title={t('quickActionsWidget.title')}>
      <div className="grid min-h-0 flex-1 auto-rows-min grid-cols-[repeat(auto-fit,minmax(150px,1fr))] content-start gap-2 overflow-hidden px-4 pb-3">
        {actions.map((a) => {
          const Icon = a.icon;
          return (
            <PlainButton
              key={a.key}
              type="button"
              disabled={a.isPending}
              onClick={() => (a.confirm ? setConfirmAction(a.key) : a.run())}
              className={cn(
                'flex min-h-0 min-w-0 flex-col items-start justify-start gap-1 overflow-hidden rounded-xl border border-hairline bg-glass px-3 py-2.5 text-left transition-colors',
                'hover:border-hairline-2 hover:bg-muted/40 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none disabled:opacity-60',
              )}
            >
              <span className="flex w-full min-w-0 items-center gap-2">
                <span className={cn('grid size-7 shrink-0 place-items-center rounded-md', TONE_SOFT[a.tone])}>
                  {a.isPending ? <Spinner size="sm" /> : <Icon className="size-4" aria-hidden="true" />}
                </span>
                <span className="min-w-0 flex-1 truncate text-sm font-medium text-foreground">{a.label}</span>
              </span>
              <span className="line-clamp-4 min-h-[4lh] w-full text-[11px] leading-snug text-muted-foreground">
                {a.hint}
              </span>
            </PlainButton>
          );
        })}
      </div>
      {active && (
        <ConfirmDialog
          open={!!confirmAction}
          onOpenChange={(isOpen) => {
            if (!isOpen) dismiss();
          }}
          title={active.label}
          description={active.confirm ?? ''}
          confirmLabel={t('quickActionsWidget.confirm')}
          variant="destructive"
          loading={active.isPending}
          onConfirm={active.run}
        />
      )}
    </WidgetFrame>
  );
}
