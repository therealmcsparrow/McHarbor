// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconArrowBackUp } from '@tabler/icons-react';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { Button } from '@resources/components/ui/Button';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import type { AgentRow } from '@resources/hooks/useAgentInventory';
import { isPrerelease } from '@resources/utils/versionOrder';
import { useReturnAgent } from '../hooks/useAgentReturn';
import { formatKeyList } from './formatKeyList';

type AgentReturnDialogProps = {
  agent: AgentRow;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

/**
 * AgentReturnDialog confirms moving a prerelease agent to stable. It needs
 * updates.manage AND environments.manage, so it reads both keys itself:
 * `<Button permission={[...]}>` is any-of and would let one key through.
 */
export function AgentReturnDialog({ agent, open: isOpen, onOpenChange }: AgentReturnDialogProps) {
  const { t, i18n } = useTranslation('settings');
  const mutation = useReturnAgent();
  const canUpdates = usePermission(PERM.UPDATES_MANAGE);
  const canEnvironments = usePermission(PERM.ENVIRONMENTS_MANAGE);
  const missing = [
    ...(canUpdates ? [] : [PERM.UPDATES_MANAGE]),
    ...(canEnvironments ? [] : [PERM.ENVIRONMENTS_MANAGE]),
  ];
  const notConnected = agent.connected === false;
  const denied = missing.length > 0 || notConnected;

  const reasonId = useId();
  const needs =
    missing.length > 0
      ? t('prerelease.return.needsMissing', { keys: formatKeyList(missing, i18n.language) })
      : t('prerelease.return.needs', {
          keys: formatKeyList([PERM.UPDATES_MANAGE, PERM.ENVIRONMENTS_MANAGE], i18n.language),
        });

  return (
    <MutationConfirmDialog
      open={isOpen}
      onOpenChange={onOpenChange}
      title={t('prerelease.return.title', { name: agent.name })}
      description={
        <>
          {t('prerelease.return.body', { version: agent.version ?? '' })}
          <span id={reasonId} className="mt-2 block text-xs">
            {notConnected && <span className="block text-destructive">{t('prerelease.notConnected')}</span>}
            {agent.pinned && <span className="block">{t('prerelease.return.alreadyPinned')}</span>}
            <span className="block">{needs}</span>
          </span>
        </>
      }
      confirmLabel={t('prerelease.return.confirm')}
      variant="caution"
      mutation={mutation}
      variables={agent.envId}
      disabled={denied}
      softDisabled
      confirmDescribedBy={denied ? reasonId : undefined}
    />
  );
}

/** AgentReturnAction is the row button plus its dialog; it stays enabled when denied so the reason can be read. */
export function AgentReturnAction({ agent }: { agent: AgentRow }) {
  const { t } = useTranslation('settings');
  const [dialogOpen, setDialogOpen] = useState(false);
  const retry = !!agent.pinned && !!agent.returnReason;
  if (!retry && !isPrerelease(agent.version)) return null;
  const label = t(retry ? 'prerelease.return.retryAria' : 'prerelease.return.rowAria', { name: agent.name });

  return (
    <>
      <Button variant="ghost" size="icon-sm" aria-label={label} onClick={() => setDialogOpen(true)}>
        <IconArrowBackUp className="size-4" aria-hidden="true" />
      </Button>
      <AgentReturnDialog agent={agent} open={dialogOpen} onOpenChange={setDialogOpen} />
    </>
  );
}
