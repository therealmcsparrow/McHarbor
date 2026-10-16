// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconPlayerPause, IconPlayerPlay, IconPlayerStop, IconPower, IconRotateClockwise } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { VM_PERM } from '../permissions';
import type { LifecycleAction } from '../hooks/useVMs';
import type { VM } from '../types';

export function GuestPowerControls({
  vm,
  busy,
  onAction,
}: {
  vm: VM;
  busy: boolean;
  onAction: (action: LifecycleAction) => void;
}) {
  const { t } = useTranslation('vm');
  const running = vm.state === 'running';
  const paused = vm.state === 'paused';

  if (!running && !paused) {
    return (
      <Button
        size="sm"
        variant="outline"
        permission={VM_PERM.MANAGE}
        disabled={busy || !vm.hostAvailable}
        onClick={() => onAction('start')}
      >
        <IconPlayerPlay className="mr-1.5 h-4 w-4" aria-hidden="true" />
        {t('actions.start')}
      </Button>
    );
  }

  return (
    <div className="flex items-center gap-1">
      <Button
        size="icon-sm"
        variant="outline"
        permission={VM_PERM.MANAGE}
        disabled={busy || !vm.hostAvailable}
        aria-label={paused ? t('actions.resume') : t('actions.pause')}
        title={paused ? t('actions.resume') : t('actions.pause')}
        onClick={() => onAction(paused ? 'resume' : 'pause')}
      >
        {paused ? (
          <IconPlayerPlay className="h-4 w-4" aria-hidden="true" />
        ) : (
          <IconPlayerPause className="h-4 w-4" aria-hidden="true" />
        )}
      </Button>
      <Button
        size="icon-sm"
        variant="outline"
        permission={VM_PERM.MANAGE}
        disabled={busy || !vm.hostAvailable}
        aria-label={t('actions.reset')}
        title={t('actions.reset')}
        onClick={() => onAction('reset')}
      >
        <IconRotateClockwise className="h-4 w-4" aria-hidden="true" />
      </Button>
      <Button
        size="icon-sm"
        variant="outline"
        permission={VM_PERM.MANAGE}
        disabled={busy || !vm.hostAvailable}
        aria-label={t('actions.shutdown')}
        title={t('actions.shutdown')}
        onClick={() => onAction('shutdown')}
      >
        <IconPower className="h-4 w-4" aria-hidden="true" />
      </Button>
      <Button
        size="icon-sm"
        variant="outline"
        permission={VM_PERM.MANAGE}
        disabled={busy || !vm.hostAvailable}
        aria-label={t('actions.stop')}
        title={t('actions.stop')}
        onClick={() => onAction('stop')}
      >
        <IconPlayerStop className="h-4 w-4 text-destructive" aria-hidden="true" />
      </Button>
    </div>
  );
}
