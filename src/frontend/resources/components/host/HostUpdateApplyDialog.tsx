// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Checkbox } from '@resources/components/ui/Checkbox';
import { Label } from '@resources/components/ui/Label';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import {
  hostPermission,
  useStartHostUpdate,
  type HostTarget,
  type HostUpdateApplyOptions,
} from '@resources/hooks/useHostUpdates';

type HostUpdateApplyDialogProps = {
  target: HostTarget;
  open: boolean;
  count: number;
  defaults?: HostUpdateApplyOptions;
  recoveryUnsupported?: boolean;
  onOpenChange: (open: boolean) => void;
  onStarted: (jobId: string) => void;
};

type OptionRowProps = {
  id: string;
  checked: boolean;
  label: string;
  description: string;
  onChange: (next: boolean) => void;
};

function OptionRow({ id, checked, label, description, onChange }: OptionRowProps) {
  return (
    <div className="flex items-start gap-3">
      <Checkbox id={id} checked={checked} onCheckedChange={(next) => onChange(next === true)} />
      <div className="min-w-0">
        <Label htmlFor={id} className="text-sm font-normal">
          {label}
        </Label>
        <p className="mt-0.5 text-xs text-muted-foreground">{description}</p>
      </div>
    </div>
  );
}

export function HostUpdateApplyDialog({
  target,
  open: isOpen,
  count,
  defaults,
  recoveryUnsupported = false,
  onOpenChange,
  onStarted,
}: HostUpdateApplyDialogProps) {
  const { t } = useTranslation('environments');
  const start = useStartHostUpdate(target);
  const baseId = useId();
  const [withNewPackages, setWithNewPackages] = useState(false);
  const [skipUnavailable, setSkipUnavailable] = useState(false);
  const [removeUnused, setRemoveUnused] = useState(false);

  const defaultsKey = [isOpen, defaults?.withNewPackages, defaults?.skipUnavailable, defaults?.removeUnused].join('|');
  const [seededKey, setSeededKey] = useState('');
  if (seededKey !== defaultsKey) {
    setSeededKey(defaultsKey);
    if (isOpen) {
      setWithNewPackages(defaults?.withNewPackages ?? false);
      setSkipUnavailable(defaults?.skipUnavailable ?? false);
      setRemoveUnused(defaults?.removeUnused ?? false);
    }
  }

  return (
    <MutationConfirmDialog
      open={isOpen}
      onOpenChange={onOpenChange}
      variant="caution"
      title={t('detail.hostUpdates.applyTitle')}
      description={t('detail.hostUpdates.applyDescription', { count })}
      confirmLabel={t('detail.hostUpdates.updateNow')}
      permission={hostPermission(target, 'updatesManage')}
      mutation={start}
      variables={{ withNewPackages, skipUnavailable, removeUnused }}
      onSuccess={(job) => {
        if (job?.jobId) onStarted(job.jobId);
      }}
    >
      <div className="space-y-3">
        <OptionRow
          id={`${baseId}-new-packages`}
          checked={withNewPackages}
          label={t('detail.hostUpdates.newPackages')}
          description={t('detail.hostUpdates.newPackagesDescription')}
          onChange={setWithNewPackages}
        />
        <OptionRow
          id={`${baseId}-skip-unavailable`}
          checked={skipUnavailable}
          label={t('detail.hostUpdates.skipUnavailable')}
          description={t('detail.hostUpdates.skipUnavailableDescription')}
          onChange={setSkipUnavailable}
        />
        <OptionRow
          id={`${baseId}-remove-unused`}
          checked={removeUnused}
          label={t('detail.hostUpdates.removeUnused')}
          description={t('detail.hostUpdates.removeUnusedDescription')}
          onChange={setRemoveUnused}
        />
        <p className="text-xs text-muted-foreground">{t('detail.hostUpdates.refreshNote')}</p>
        <p className="text-xs text-muted-foreground">{t('detail.hostUpdates.applyDefaultsNote')}</p>
        {recoveryUnsupported ? (
          <p className="text-xs text-warning">{t('detail.hostUpdates.recoveryUnsupported')}</p>
        ) : null}
      </div>
    </MutationConfirmDialog>
  );
}
