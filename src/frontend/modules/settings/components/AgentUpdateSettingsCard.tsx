// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// AgentUpdateSettingsCard — everything about keeping *remote agents* current:
// the auto-update toggle, the version the host would push, the per-environment
// exception list, and the manual sweep. Split out of `AutoUpdateSettingsCard`
// when Updates became its own settings group, so the Agents section stands on
// its own instead of sharing a card with McHarbor's self-install.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconRefresh, IconServer, IconShieldOff } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { PERM } from '@core/auth/permissionKeys';
import {
  useSaveSelfUpdateSettings,
  useSelfUpdateSettings,
  useSelfUpdateState,
  useUpdateAgentsNow,
  type SelfUpdateSettings,
} from '../hooks/useUpdates';
import { AgentUpdateExceptionsDialog } from './AgentUpdateExceptionsDialog';
import { GatedToggleRow } from './GatedToggleRow';

export function AgentUpdateSettingsCard() {
  const { t } = useTranslation('settings');
  const { data: settings, isFetching } = useSelfUpdateSettings();
  const { data: state } = useSelfUpdateState();
  const save = useSaveSelfUpdateSettings();
  const updateAgents = useUpdateAgentsNow();
  const [exceptionsOpen, setExceptionsOpen] = useState(false);

  if (!settings) {
    return null;
  }

  const current = settings;
  const busy = isFetching || save.isPending;

  function update(patch: Partial<SelfUpdateSettings>) {
    save.mutate({ ...current, ...patch });
  }

  return (
    <div className="rounded-lg border border-border p-5">
      <div className="mb-4 flex items-start gap-3">
        <span className="flex size-9 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
          <IconServer className="size-4" />
        </span>
        <div>
          <h3 className="text-sm font-semibold text-foreground">{t('about.autoUpdate.agentsTitle')}</h3>
          <p className="text-xs text-muted-foreground">{t('about.autoUpdate.agentsSubtitle')}</p>
        </div>
      </div>

      <div className="space-y-4">
        <GatedToggleRow
          label={t('about.autoUpdate.agentsLabel')}
          hint={t('about.autoUpdate.agentsHint')}
          checked={settings.autoUpdateAgents}
          disabled={busy || !settings.enabled}
          permission={PERM.ENVIRONMENTS_MANAGE}
          onChange={(v) => update({ autoUpdateAgents: v })}
        />

        <div className="flex flex-wrap items-center justify-between gap-3 rounded-md bg-muted/40 px-3 py-2">
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            <IconServer className="size-3.5" />
            {state?.latestAgentVersion
              ? t('about.autoUpdate.agentTarget', { version: state.latestAgentVersion })
              : t('about.autoUpdate.agentTargetUnknown')}
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <Button
              variant="ghost"
              size="sm"
              permission={PERM.ENVIRONMENTS_MANAGE}
              onClick={() => setExceptionsOpen(true)}
              disabled={busy}
            >
              <IconShieldOff className="size-4" />
              {t('about.autoUpdate.exceptions.button', { n: settings.agentExcludedEnvIds.length })}
            </Button>
            <Button
              variant="outline"
              size="sm"
              permission={PERM.ENVIRONMENTS_MANAGE}
              onClick={() => updateAgents.mutate()}
              disabled={updateAgents.isPending}
            >
              {updateAgents.isPending ? <Spinner size="sm" /> : <IconRefresh className="size-4" />}
              {t('about.autoUpdate.updateAgentsNow')}
            </Button>
          </div>
        </div>
        <AgentUpdateExceptionsDialog
          open={exceptionsOpen}
          onOpenChange={setExceptionsOpen}
          excluded={settings.agentExcludedEnvIds}
          saving={save.isPending}
          onSave={(ids) => {
            save.mutate({ ...current, agentExcludedEnvIds: ids }, { onSuccess: () => setExceptionsOpen(false) });
          }}
        />
        {updateAgents.data && (
          <p className="text-xs text-muted-foreground">
            {t('about.autoUpdate.agentSweepQueued', {
              count: updateAgents.data.queued,
              target: updateAgents.data.targetVersion,
            })}
          </p>
        )}
        {!settings.enabled && <p className="text-xs text-muted-foreground">{t('about.autoUpdate.checkerDisabled')}</p>}
      </div>
    </div>
  );
}
