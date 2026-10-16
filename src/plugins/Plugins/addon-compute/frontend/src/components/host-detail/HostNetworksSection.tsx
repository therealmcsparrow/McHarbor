// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconNetwork, IconPencil, IconPlus, IconTrash } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Card } from '@resources/components/ui/Card';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { HeaderActionButton } from '@resources/components/ui/HeaderActionButton';
import { StatusError } from '@resources/components/ui/StatusError';
import { COMPUTE_PERM } from '../../permissions';
import { useDeleteNetwork, useNetworks } from '../../hooks/useCompute';
import { CreateNetworkDialog } from '../CreateNetworkDialog';
import type { Network, OverviewEntry } from '../../types';

interface HostNetworksSectionProps {
  entry: OverviewEntry;
}

export function HostNetworksSection({ entry }: HostNetworksSectionProps) {
  const { t } = useTranslation('compute');
  const { data: networks } = useNetworks(entry.environmentId);
  const remove = useDeleteNetwork();
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<Network | null>(null);
  const [deleting, setDeleting] = useState<Network | null>(null);

  const rows = networks ?? [];
  const environments = [{ id: entry.environmentId, name: entry.name }];

  return (
    <Card className="space-y-4 p-5">
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-sm font-medium text-foreground">{t('nav.networks')}</h2>
        <Button size="sm" permission={COMPUTE_PERM.MANAGE} onClick={() => setCreating(true)}>
          <IconPlus className="h-4 w-4" aria-hidden="true" />
          {t('actions.createNetwork')}
        </Button>
      </div>

      {rows.length === 0 ? (
        <EmptyState
          icon={<IconNetwork className="size-6" />}
          title={t('hosts.settings.noNetworks.title')}
          description={t('hosts.settings.noNetworks.description')}
        />
      ) : (
        <ul className="space-y-2">
          {rows.map((network) => (
            <li key={network.id} className="flex flex-wrap items-center gap-2 rounded-lg border border-border p-3">
              <span className="truncate text-sm font-medium text-foreground">{network.name}</span>
              <Badge variant="default" className="px-1.5 py-0 text-[10px]">
                {t(`networks.mode.${network.mode}`)}
              </Badge>
              <span className="truncate font-mono text-xs text-muted-foreground">{network.bridge}</span>
              <StatusError error={network.lastError} />
              <div className="ml-auto flex items-center gap-1">
                <HeaderActionButton
                  tooltip={t('actions.edit')}
                  icon={<IconPencil className="size-4" />}
                  permission={COMPUTE_PERM.MANAGE}
                  onClick={() => setEditing(network)}
                />
                <HeaderActionButton
                  tooltip={t('actions.delete')}
                  icon={<IconTrash className="size-4" />}
                  permission={COMPUTE_PERM.DELETE}
                  disabled={remove.isPending}
                  onClick={() => setDeleting(network)}
                />
              </div>
            </li>
          ))}
        </ul>
      )}

      {(creating || editing) && (
        <CreateNetworkDialog
          environments={environments}
          defaultEnvironmentId={entry.environmentId}
          network={editing ?? undefined}
          onClose={() => {
            setCreating(false);
            setEditing(null);
          }}
        />
      )}

      {deleting && (
        <ConfirmDialog
          open
          onOpenChange={(isOpen) => !isOpen && setDeleting(null)}
          title={t('networks.delete.title', { name: deleting.name })}
          description={t(`networks.delete.${deleting.mode}Body`)}
          confirmLabel={t('networks.delete.submit')}
          variant="destructive"
          onConfirm={() => {
            remove.mutate(deleting.id, { onSuccess: () => setDeleting(null) });
          }}
        />
      )}
    </Card>
  );
}
