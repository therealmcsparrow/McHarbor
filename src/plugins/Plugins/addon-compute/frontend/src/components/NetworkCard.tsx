// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAddressBook, IconPencil, IconServer, IconTrash } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Card, CardContent, CardFooter } from '@resources/components/ui/Card';
import { attachmentLabel, hostLinkLabel } from '../utils/networkAttachment';
import { COMPUTE_PERM } from '../permissions';
import { StatTile } from './StatTile';
import type { Network } from '../types';

interface NetworkCardProps {
  network: Network;
  hostName: string;
  onViewLeases: (network: Network) => void;
  onEdit: (network: Network) => void;
  onDelete: (network: Network) => void;
}

/** One guest network, in the same shape as an environment card. */
export function NetworkCard({ network, hostName, onViewLeases, onEdit, onDelete }: NetworkCardProps) {
  const { t } = useTranslation('compute');

  return (
    <Card className="min-h-[300px] transition-colors hover:border-primary/40">
      <CardContent className="flex flex-1 flex-col gap-4 p-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0 space-y-1">
            <div className="flex min-w-0 items-center gap-2">
              <h3 className="truncate text-base font-semibold text-foreground">{network.name}</h3>
              {network.isDefault && (
                <Badge variant="secondary" className="shrink-0 px-2 py-1 text-[10px]">
                  {t('badges.default')}
                </Badge>
              )}
            </div>
            <p className="flex items-center gap-1.5 truncate text-xs text-muted-foreground">
              <IconServer className="size-3.5 shrink-0" aria-hidden="true" />
              {hostName}
            </p>
          </div>
          <Badge variant={network.mode === 'managed' ? 'default' : 'outline'} className="shrink-0">
            {t(`networks.mode.${network.mode}`)}
          </Badge>
        </div>

        <div className="rounded-lg border border-border/70 bg-background/40 px-3 py-2">
          <p className="text-[11px] font-medium text-muted-foreground">{t('networks.columns.attachment')}</p>
          <code className="mt-1 block truncate font-mono text-xs text-foreground">{attachmentLabel(network)}</code>
          {hostLinkLabel(network) && (
            <>
              <p className="mt-2 text-[11px] font-medium text-muted-foreground">
                {t('networks.columns.hostInterface')}
              </p>
              <code className="mt-1 block truncate font-mono text-xs text-foreground">{hostLinkLabel(network)}</code>
            </>
          )}
        </div>

        {network.lastError && <p className="text-xs break-words text-destructive">{network.lastError}</p>}

        <div className="mt-auto grid grid-cols-2 gap-2">
          <StatTile
            label={t('networks.columns.subnet')}
            value={network.subnetCidr || '—'}
            tone={network.subnetCidr ? 'default' : 'muted'}
          />
          <StatTile
            label={t('networks.create.gateway')}
            value={network.gatewayIp || '—'}
            tone={network.gatewayIp ? 'default' : 'muted'}
          />
        </div>
      </CardContent>

      <CardFooter className="gap-1 px-3 py-2">
        {network.mode === 'managed' && (
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t('actions.viewLeases')}
            title={t('actions.viewLeases')}
            permission={COMPUTE_PERM.VIEW}
            onClick={() => onViewLeases(network)}
          >
            <IconAddressBook className="h-4 w-4" aria-hidden="true" />
          </Button>
        )}
        <div className="flex-1" />
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={t('actions.edit')}
          title={t('actions.edit')}
          permission={COMPUTE_PERM.MANAGE}
          onClick={() => onEdit(network)}
        >
          <IconPencil className="h-4 w-4" aria-hidden="true" />
        </Button>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={t('actions.delete')}
          title={t('actions.delete')}
          permission={COMPUTE_PERM.DELETE}
          onClick={() => onDelete(network)}
        >
          <IconTrash className="h-4 w-4 text-destructive" aria-hidden="true" />
        </Button>
      </CardFooter>
    </Card>
  );
}
