// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconTrash, IconPhoto } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { usePruneImages } from '@modules/images/hooks/useImages';
import { usePruneVolumes } from '@modules/volumes/hooks/useVolumes';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';

type ActionKey = 'pruneImages' | 'pruneVolumes';

export default function QuickActionsWidget({ typeId: _typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const [confirmAction, setConfirmAction] = useState<ActionKey | null>(null);

  const pruneImages = usePruneImages();
  const pruneVolumes = usePruneVolumes();

  const actions: { key: ActionKey; icon: typeof IconTrash; label: string; description: string; onConfirm: () => void; isPending: boolean }[] = [
    {
      key: 'pruneImages',
      icon: IconPhoto,
      label: t('quickActionsWidget.pruneImages'),
      description: t('quickActionsWidget.pruneImagesDesc'),
      onConfirm: () => {
        pruneImages.mutate(undefined, { onSettled: () => setConfirmAction(null) });
      },
      isPending: pruneImages.isPending,
    },
    {
      key: 'pruneVolumes',
      icon: IconTrash,
      label: t('quickActionsWidget.pruneVolumes'),
      description: t('quickActionsWidget.pruneVolumesDesc'),
      onConfirm: () => {
        pruneVolumes.mutate(undefined, { onSettled: () => setConfirmAction(null) });
      },
      isPending: pruneVolumes.isPending,
    },
  ];

  const activeAction = actions.find((a) => a.key === confirmAction);

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <h3 className="shrink-0 px-4 pt-3 pb-2 text-sm font-semibold text-foreground">
        {t('quickActionsWidget.title')}
      </h3>
      <div className="flex flex-1 flex-wrap gap-2 px-4 pb-3 content-start">
        {actions.map((a) => {
          const Icon = a.icon;
          return (
            <Button
              key={a.key}
              variant="outline"
              size="sm"
              className="gap-1.5"
              onClick={() => setConfirmAction(a.key)}
            >
              <Icon className="h-3.5 w-3.5" />
              {a.label}
            </Button>
          );
        })}
      </div>
      {activeAction && (
        <ConfirmDialog
          open={!!confirmAction}
          onOpenChange={(open) => { if (!open) setConfirmAction(null); }}
          title={activeAction.label}
          description={activeAction.description}
          confirmLabel={t('quickActionsWidget.confirm')}
          variant="destructive"
          loading={activeAction.isPending}
          onConfirm={activeAction.onConfirm}
        />
      )}
    </div>
  );
}
