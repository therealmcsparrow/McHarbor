// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconPlayerPlay, IconPlayerStop, IconSnowflake, IconTerminal2 } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { CT_PERM } from '../permissions';
import type { LifecycleAction } from '../hooks/useContainers';
import { isLive, type Container } from '../types';

/**
 * The container's address.
 *
 * Unlike addon-kvm's equivalent this is not a guest claim: the agent reads it
 * from the container's network namespace, so it is what the host observed.
 * Still rendered as text and never as a link — a link here would invite a click
 * into whatever is listening inside the container.
 */
export function ObservedAddress({ container }: { container: Container }) {
  const { t } = useTranslation('ct');
  const address = container.observedIps?.[0];

  if (!address) {
    return <span className="text-muted-foreground">—</span>;
  }

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="font-mono text-xs text-muted-foreground">{address}</span>
      </TooltipTrigger>
      <TooltipContent>{t('address.observedHint')}</TooltipContent>
    </Tooltip>
  );
}

export function RowActions({
  container,
  busy,
  onOpen,
  onAction,
}: {
  container: Container;
  busy: boolean;
  onOpen: (id: string) => void;
  onAction: (action: LifecycleAction) => void;
}) {
  const { t } = useTranslation('ct');

  // A container whose host cannot be reached gets disabled actions rather than
  // hidden ones — the operator should see that the control exists and why it
  // will not work.
  const disabled = busy || !container.hostAvailable;

  return (
    <div className="flex justify-end gap-1">
      {isLive(container) ? (
        <>
          <Button
            variant="ghost"
            size="icon"
            aria-label={t('actions.console')}
            permission={CT_PERM.CONSOLE}
            disabled={disabled}
            onClick={() => onOpen(container.id)}
          >
            <IconTerminal2 className="h-4 w-4" aria-hidden="true" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            aria-label={container.state === 'frozen' ? t('actions.unfreeze') : t('actions.freeze')}
            permission={CT_PERM.MANAGE}
            disabled={disabled}
            onClick={() => onAction(container.state === 'frozen' ? 'unfreeze' : 'freeze')}
          >
            <IconSnowflake className="h-4 w-4" aria-hidden="true" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            aria-label={t('actions.shutdown')}
            permission={CT_PERM.MANAGE}
            disabled={disabled}
            onClick={() => onAction('shutdown')}
          >
            <IconPlayerStop className="h-4 w-4" aria-hidden="true" />
          </Button>
        </>
      ) : (
        <Button
          variant="ghost"
          size="icon"
          aria-label={t('actions.start')}
          permission={CT_PERM.MANAGE}
          // A template is deliberately not startable: that immutability is what
          // makes it a template.
          disabled={disabled || container.isTemplate}
          onClick={() => onAction('start')}
        >
          <IconPlayerPlay className="h-4 w-4" aria-hidden="true" />
        </Button>
      )}
    </div>
  );
}
