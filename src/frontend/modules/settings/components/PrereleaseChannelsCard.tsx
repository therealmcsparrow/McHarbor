// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconFlask } from '@tabler/icons-react';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { Spinner } from '@resources/components/ui/Spinner';
import { Switch } from '@resources/components/ui/Switch';
import { useSetUpdateChannel, useUpdateChannels, type UpdateChannelPatch } from '@resources/hooks/useUpdateChannels';
import { isPrerelease } from '@resources/utils/versionOrder';
import { useSelfUpdateSettings, useSelfUpdateState } from '../hooks/useUpdates';

type Scope = 'mcharbor' | 'agents';

type RowProps = {
  label: string;
  hint: string;
  checked: boolean;
  locked: boolean;
  busy: boolean;
  reasons: string[];
  indented?: boolean;
  onChange: (next: boolean) => void;
};

/**
 * A denied or blocked row keeps the switch focusable (`aria-disabled`) and
 * carries every reason as visible text linked by `aria-describedby`, so a
 * touch or screen-reader user reads why, not only a tooltip.
 */
function ChannelRow({ label, hint, checked, locked, busy, reasons, indented, onChange }: RowProps) {
  const base = useId();
  const describedBy = [`${base}-hint`, ...reasons.map((_, i) => `${base}-reason-${i}`)].join(' ');
  return (
    <div className={indented ? 'ml-6 space-y-1 border-l border-border pl-4' : 'space-y-1'}>
      <div className="flex items-center justify-between gap-4">
        <p id={`${base}-label`} className="text-sm font-medium text-foreground">
          {label}
        </p>
        <span className="flex items-center gap-2">
          {busy && <Spinner size="sm" />}
          <Switch
            checked={checked}
            aria-labelledby={`${base}-label`}
            aria-describedby={describedBy}
            aria-disabled={locked || busy}
            aria-busy={busy}
            onCheckedChange={(next) => {
              if (locked || busy) return;
              onChange(next);
            }}
          />
        </span>
      </div>
      <p id={`${base}-hint`} className="text-xs text-muted-foreground">
        {hint}
      </p>
      {reasons.map((reason, i) => (
        <p key={reason} id={`${base}-reason-${i}`} className="text-xs text-muted-foreground">
          {reason}
        </p>
      ))}
    </div>
  );
}

/**
 * PrereleaseChannelsCard holds the two prerelease switches of one update
 * scope (McHarbor or agents). Each flip sends only its own field, so a stale
 * tab cannot overwrite a switch changed elsewhere.
 */
export function PrereleaseChannelsCard({ scope }: { scope: Scope }) {
  const { t } = useTranslation('settings');
  const canManage = usePermission(PERM.UPDATES_MANAGE);
  const { canView, settings, isLoading, isError, refetch } = useUpdateChannels();
  const { data: updateSettings } = useSelfUpdateSettings();
  const { data: state } = useSelfUpdateState();
  const save = useSetUpdateChannel();
  const [confirmAuto, setConfirmAuto] = useState(false);

  const agents = scope === 'agents';
  const parentKey = agents ? 'agentPrereleases' : 'mcharborPrereleases';
  const autoKey = agents ? 'agentAutoPrereleases' : 'mcharborAutoPrereleases';
  const g = agents ? 'prerelease.agents' : 'prerelease.mcharbor';

  if (!canView) {
    return (
      <div className="rounded-lg border border-border p-5">
        <h3 className="text-sm font-semibold text-foreground">{t(`${g}.title`)}</h3>
        <p className="mt-2 text-xs text-muted-foreground">{t('prerelease.needsView', { key: PERM.UPDATES_VIEW })}</p>
      </div>
    );
  }

  const busy = isLoading || save.isPending;
  const autoParentOn = agents ? updateSettings?.autoUpdateAgents === true : updateSettings?.autoInstall === true;
  const parentOn = settings?.[parentKey] === true;
  const manageReason = canManage ? [] : [t('prerelease.needs', { key: PERM.UPDATES_MANAGE })];
  const autoSwitchReason = parentOn && autoParentOn ? [] : [t(`${g}.autoNeeds`)];
  const version = agents ? state?.latestAgentVersion : state?.currentVersion;
  // The McHarbor text lives in the status region and the update card; only the agent card carries its own.
  const showRunning = agents && !!settings && !parentOn && isPrerelease(version);

  function send(patch: UpdateChannelPatch) {
    save.mutate(patch);
  }

  return (
    <div className="rounded-lg border border-border p-5" aria-busy={busy}>
      <div className="mb-4 flex items-start gap-3">
        <span className="flex size-9 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
          <IconFlask className="size-4" />
        </span>
        <div>
          <h3 className="text-sm font-semibold text-foreground">{t(`${g}.title`)}</h3>
          <p className="text-xs text-muted-foreground">{t('prerelease.subtitle')}</p>
        </div>
      </div>

      {showRunning && (
        <output className="mb-4 block rounded-md bg-muted/40 px-3 py-2 text-xs text-foreground">
          {t(`${g}.running`, { version })}
        </output>
      )}

      {isError ? (
        <div role="alert" className="flex items-center gap-3 text-xs text-destructive">
          <span>{t('prerelease.loadFailed')}</span>
          <Button size="sm" variant="secondary" onClick={() => void refetch()}>
            {t('prerelease.retry')}
          </Button>
        </div>
      ) : (
        <div className="space-y-4">
          <ChannelRow
            label={t(`${g}.label`)}
            hint={t(`${g}.hint`)}
            checked={parentOn}
            locked={!canManage}
            busy={busy}
            reasons={manageReason}
            onChange={(next) => send({ [parentKey]: next })}
          />
          <ChannelRow
            indented
            label={t(`${g}.autoLabel`)}
            hint={t(`${g}.autoHint`)}
            checked={settings?.[autoKey] === true}
            locked={!canManage || !parentOn || !autoParentOn}
            busy={busy}
            reasons={[...manageReason, ...autoSwitchReason]}
            onChange={(next) => (next ? setConfirmAuto(true) : send({ [autoKey]: false }))}
          />
        </div>
      )}

      <ConfirmDialog
        open={confirmAuto}
        onOpenChange={setConfirmAuto}
        title={t('prerelease.confirmAuto.title')}
        description={t(`${g}.confirmAuto`)}
        confirmLabel={t('prerelease.confirmAuto.confirm')}
        variant="caution"
        loading={save.isPending}
        onConfirm={() => save.mutate({ [autoKey]: true }, { onSettled: () => setConfirmAuto(false) })}
      />
    </div>
  );
}
