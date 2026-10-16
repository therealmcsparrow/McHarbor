// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconInfoCircle, IconRefresh, IconShieldOff } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { Spinner } from '@resources/components/ui/Spinner';
import { formatDate } from '@resources/utils/format';
import { COMPUTE_PERM } from '../permissions';
import { useProbe } from '../hooks/useCompute';
import { CapabilityRow, ProbeFailure } from './CapabilityRow';
import type { Capability, UnavailableReason } from '../types';

/**
 * The node capability panel.
 *
 * The roadmap sets the standard this component has to hit: "`/dev/kvm` missing —
 * your CPU reports VT-x but it is disabled in firmware" is the target, not
 * "capability check failed". So every failed check renders three things — what
 * is missing, what was actually observed, and a copyable command that fixes it
 * on this host's own distribution.
 *
 * The panel is also honest about scope. A host with no `/dev/kvm` can still run
 * system containers, and a host on cgroup v1 can still run virtual machines, so
 * items say what they block rather than collapsing into one pass/fail.
 *
 * That honesty has to cover the check itself failing. The plugin runs its own
 * `QueryClient` — it compiles a private copy of TanStack Query — so the host's
 * global mutation-error toast never fires here, and `sonner` cannot cross the
 * bundle boundary either. A refused probe therefore has to be rendered in this
 * panel or it is not rendered anywhere, which is how "check this host again"
 * came to look like a button that did nothing.
 */

interface CapabilityPanelProps {
  environmentId: string;
  capability?: Capability;
  probedAt?: string;
  available: boolean;
  unavailableReason?: UnavailableReason;
  agentVersionRequired?: string;
  agentVersionCurrent?: string;
}

export function CapabilityPanel({
  environmentId,
  capability,
  probedAt,
  available,
  unavailableReason,
  agentVersionRequired = '3.0.0',
  agentVersionCurrent = '',
}: CapabilityPanelProps) {
  const { t } = useTranslation('compute');
  const probe = useProbe();

  if (!available) {
    return (
      <UnavailableNotice reason={unavailableReason} required={agentVersionRequired} current={agentVersionCurrent} />
    );
  }

  if (!capability) {
    return (
      <div className="rounded-lg border border-border bg-card p-6 text-center">
        <p className="text-sm text-muted-foreground">{t('capability.neverProbed')}</p>
        <Button
          className="mt-4"
          permission={COMPUTE_PERM.VIEW}
          onClick={() => probe.mutate(environmentId)}
          disabled={probe.isPending}
        >
          {probe.isPending ? (
            <Spinner className="mr-2 h-4 w-4" />
          ) : (
            <IconRefresh className="mr-2 h-4 w-4" aria-hidden="true" />
          )}
          {probe.isPending ? t('actions.probing') : t('actions.probe')}
        </Button>
        {probe.isError && <ProbeFailure error={probe.error} className="mt-4 text-left" />}
      </div>
    );
  }

  // A host that cannot run guests at all gets one clear explanation rather
  // than a checklist of things it will never have.
  if (!capability.supported) {
    return (
      <div className="rounded-lg border border-border bg-card p-6">
        <div className="flex items-start gap-3">
          <IconInfoCircle className="mt-0.5 h-5 w-5 shrink-0 text-muted-foreground" aria-hidden="true" />
          <div>
            <h3 className="font-medium text-foreground">{t('capability.unsupported.title')}</h3>
            <p className="mt-1 text-sm text-muted-foreground">
              {capability.unsupportedWhy || t('capability.unsupported.body')}
            </p>
          </div>
        </div>
      </div>
    );
  }

  const items = capability.items ?? [];
  const blockers = items.filter((item) => item.critical && !item.present);

  return (
    <div className="rounded-lg border border-border bg-card">
      <header className="flex flex-wrap items-center justify-between gap-3 border-b border-border p-4">
        <div>
          <h3 className="font-medium text-foreground">{t('capability.title')}</h3>
          <p className="mt-0.5 text-sm text-muted-foreground">{t('capability.subtitle')}</p>
        </div>
        <div className="flex items-center gap-2">
          <ConclusionBadge
            ok={capability.canRunVms}
            label={t('capability.canRunVms')}
            failLabel={t('capability.cannotRunVms')}
          />
          <ConclusionBadge
            ok={capability.canRunContainers}
            label={t('capability.canRunContainers')}
            failLabel={t('capability.cannotRunContainers')}
          />
          <Button
            variant="ghost"
            size="icon"
            aria-label={probe.isPending ? t('actions.probing') : t('actions.probe')}
            permission={COMPUTE_PERM.VIEW}
            onClick={() => probe.mutate(environmentId)}
            disabled={probe.isPending}
          >
            {probe.isPending ? <Spinner className="h-4 w-4" /> : <IconRefresh className="h-4 w-4" aria-hidden="true" />}
          </Button>
        </div>
      </header>

      {probe.isError && <ProbeFailure error={probe.error} className="m-4" />}

      {/*
        The seccomp state is a first-class notice rather than a checklist line.
        Built without the tag, containers get no syscall filter at all — user
        namespace isolation still applies, but an operator who assumed a filter
        was there deserves to be told plainly, not to find it in a list.
      */}
      {!capability.seccompBuiltIn && (
        <div className="flex items-start gap-3 border-b border-border bg-warning/10 p-4">
          <IconShieldOff className="mt-0.5 h-5 w-5 shrink-0 text-warning" aria-hidden="true" />
          <div>
            <p className="text-sm font-medium text-foreground">{t('capability.seccompOff.title')}</p>
            <p className="mt-0.5 text-sm text-muted-foreground">{t('capability.seccompOff.body')}</p>
          </div>
        </div>
      )}

      {capability.supervisorMode === 'systemd-run' && (
        <div className="flex items-start gap-3 border-b border-border p-4">
          <IconInfoCircle className="mt-0.5 h-5 w-5 shrink-0 text-muted-foreground" aria-hidden="true" />
          <p className="text-sm text-muted-foreground">{t('capability.supervisor.systemd-run')}</p>
        </div>
      )}

      <ul className="divide-y divide-border">
        {items.map((item) => (
          <CapabilityRow key={item.key} item={item} environmentId={environmentId} />
        ))}
      </ul>

      <footer className="flex flex-wrap items-center justify-between gap-2 border-t border-border p-3 text-xs text-muted-foreground">
        <span>
          {capability.os} · {capability.arch}
          {capability.kernelVersion ? ` · ${capability.kernelVersion}` : ''}
          {capability.agentVersion ? ` · agent ${capability.agentVersion}` : ''}
        </span>
        {probedAt && <span>{t('capability.probedAt', { when: formatDate(probedAt) })}</span>}
      </footer>

      {blockers.length > 0 && (
        <div className="border-t border-border bg-destructive/10 p-3 text-sm text-foreground">
          {t('wizard.capability.blockers', { count: blockers.length })}
        </div>
      )}
    </div>
  );
}

function ConclusionBadge({ ok, label, failLabel }: { ok: boolean; label: string; failLabel: string }) {
  return <Badge variant={ok ? 'success' : 'destructive'}>{ok ? label : failLabel}</Badge>;
}

/**
 * Why a host cannot be reached.
 *
 * The three reasons have three different fixes — connect an agent, bring the
 * agent back, update the agent — so they are three different messages. One
 * "unavailable" would leave the operator without the thing they need to know.
 */
// Exported so the image catalog can say the same sentence. A host that cannot
// be reached cannot take a download either, and two pages inventing two
// wordings for one condition is how they drift apart.
export function UnavailableNotice({
  reason,
  required,
  current,
}: {
  reason?: UnavailableReason;
  required: string;
  current: string;
}) {
  const { t } = useTranslation('compute');

  const content =
    reason === 'not_agent_environment'
      ? { title: t('agent.required.title'), body: t('agent.required.body') }
      : reason === 'agent_too_old'
        ? {
            title: t('agent.tooOld.title'),
            body: t('agent.tooOld.body', { required, current: current || '—' }),
          }
        : { title: t('agent.offline.title'), body: t('agent.offline.body') };

  return (
    <div className="rounded-lg border border-border bg-card p-6">
      <div className="flex items-start gap-3">
        <IconAlertTriangle className="mt-0.5 h-5 w-5 shrink-0 text-warning" aria-hidden="true" />
        <div>
          <h3 className="font-medium text-foreground">{content.title}</h3>
          <p className="mt-1 text-sm text-muted-foreground">{content.body}</p>
        </div>
      </div>
    </div>
  );
}
