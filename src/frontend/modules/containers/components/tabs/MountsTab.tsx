// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconFolderOpen, IconListDetails } from '@tabler/icons-react';
import type { ContainerInspect, ContainerMount } from '@core/types/docker';
import { Button } from '@resources/components/ui/Button';
import { InfoRow } from '@resources/components/ui/InfoRow';
import { ContextDrawer } from '../ContextDrawer';
import { MountsMobileList } from '../MountsMobileList';
import type { EditFormData } from '../../types/edit-form';
import { MountsEditor } from './MountsEditor';

type MountsTabProps = {
  container: ContainerInspect;
  onBrowseFiles?: () => void;
  editing?: boolean;
  editData?: EditFormData | null;
  onFieldChange?: <K extends keyof EditFormData>(field: K, value: EditFormData[K]) => void;
};

export function MountsTab({ container, onBrowseFiles, editing = false, editData, onFieldChange }: MountsTabProps) {
  const { t } = useTranslation('containers');
  const mounts = container.Mounts ?? [];
  const [selectedMount, setSelectedMount] = useState<ContainerMount | null>(null);
  const [mountsFor, setMountsFor] = useState(container.Id);
  if (mountsFor !== container.Id) {
    setMountsFor(container.Id);
    setSelectedMount(null);
  }

  if (editing && editData) {
    return (
      <div className="mc-glass rounded-lg p-6">
        <MountsEditor mounts={editData.mounts} onChange={(next) => onFieldChange?.('mounts', next)} />
      </div>
    );
  }

  if (mounts.length === 0) {
    return (
      <div className="mc-glass rounded-lg p-6">
        <p className="text-sm text-muted-foreground">{t('mounts.noMounts')}</p>
      </div>
    );
  }

  return (
    <div className="mc-glass rounded-lg p-6">
      <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
        {t('detail.mounts')}
      </h3>
      <MountsMobileList mounts={mounts} onSelect={setSelectedMount} />
      <div className="hidden overflow-x-auto md:block">
        <table className="w-full text-left text-xs">
          <thead className="bg-muted">
            <tr className="border-b border-border">
              <th scope="col" className="px-3 py-2 font-medium text-muted-foreground">
                {t('mounts.type')}
              </th>
              <th scope="col" className="px-3 py-2 font-medium text-muted-foreground">
                {t('mounts.source')}
              </th>
              <th scope="col" className="px-3 py-2 font-medium text-muted-foreground">
                {t('mounts.destination')}
              </th>
              <th scope="col" className="px-3 py-2 font-medium text-muted-foreground">
                {t('mounts.mode')}
              </th>
              <th scope="col" className="px-3 py-2 font-medium text-muted-foreground">
                {t('mounts.driver')}
              </th>
              <th scope="col" className="px-3 py-2 font-medium text-muted-foreground">
                {t('mounts.propagation')}
              </th>
              <th scope="col" className="w-12 px-3 py-2">
                <span className="sr-only">{t('mounts.actions')}</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {mounts.map((m) => (
              <tr
                key={`${m.Type}:${m.Source ?? ''}:${m.Destination}`}
                className="border-b border-border last:border-0 hover:bg-muted/50"
              >
                <td className="px-3 py-2">
                  <span className="rounded-md bg-muted px-1.5 py-0.5 text-[10px] font-medium text-foreground">
                    {m.Type}
                  </span>
                </td>
                <td className="max-w-xs truncate px-3 py-2 font-mono text-foreground">{m.Source || '-'}</td>
                <td className="max-w-xs truncate px-3 py-2 font-mono text-foreground">{m.Destination}</td>
                <td className="px-3 py-2 text-foreground">{m.RW ? t('mounts.accessRw') : t('mounts.accessRo')}</td>
                <td className="px-3 py-2 text-foreground">{m.Driver || '-'}</td>
                <td className="px-3 py-2 text-foreground">{m.Propagation || '-'}</td>
                <td className="px-3 py-2 text-right">
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    onClick={() => setSelectedMount(m)}
                    aria-label={t('mounts.viewDetails', {
                      destination: m.Destination,
                    })}
                    aria-haspopup="dialog"
                    className="size-11 sm:size-9"
                  >
                    <IconListDetails className="size-4" />
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <ContextDrawer
        open={selectedMount !== null}
        onOpenChange={(isOpen) => !isOpen && setSelectedMount(null)}
        title={t('mounts.detailsTitle')}
        description={t('mounts.detailsDescription', {
          destination: selectedMount?.Destination ?? '',
        })}
        footer={
          onBrowseFiles ? (
            <Button
              onClick={() => {
                setSelectedMount(null);
                onBrowseFiles();
              }}
            >
              <IconFolderOpen className="size-4" />
              {t('mounts.browseFiles')}
            </Button>
          ) : undefined
        }
      >
        {selectedMount ? (
          <div className="space-y-1">
            <InfoRow label={t('mounts.type')}>{selectedMount.Type}</InfoRow>
            {selectedMount.Name ? <InfoRow label={t('mounts.name')}>{selectedMount.Name}</InfoRow> : null}
            <InfoRow label={t('mounts.source')}>
              <span className="font-mono text-xs break-all">{selectedMount.Source || '-'}</span>
            </InfoRow>
            <InfoRow label={t('mounts.destination')}>
              <span className="font-mono text-xs break-all">{selectedMount.Destination}</span>
            </InfoRow>
            <InfoRow label={t('mounts.mode')}>{selectedMount.Mode || '-'}</InfoRow>
            <InfoRow label={t('mounts.access')}>
              {selectedMount.RW ? t('mounts.readWrite') : t('mounts.readOnly')}
            </InfoRow>
            <InfoRow label={t('mounts.driver')}>{selectedMount.Driver || '-'}</InfoRow>
            <InfoRow label={t('mounts.propagation')}>{selectedMount.Propagation || '-'}</InfoRow>
          </div>
        ) : null}
      </ContextDrawer>
    </div>
  );
}
