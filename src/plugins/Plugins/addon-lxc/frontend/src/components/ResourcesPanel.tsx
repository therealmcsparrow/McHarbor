// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconBolt, IconInfoCircle } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { CT_PERM } from '../permissions';
import { useContainerStats, useUpdateLimits } from '../hooks/useContainers';
import { formatBytes } from '../format';
import { BytesField, CPU_UNLIMITED, Meter, percentLabel, share } from './ResourceMeters';
import { isLive, limitsApplyLive, type Container, type Limits } from '../types';

/**
 * A container's resource limits, edited in place.
 *
 * This panel is the reason this plugin exists as something other than a copy of
 * addon-kvm. cgroup v2 applies these values to a *running* container: changing
 * a VM's memory means a reboot, changing a container's means one write to
 * `memory.max` and it is live. So the panel edits in place, applies without a
 * restart, and says so.
 *
 * Two consequences that shape everything below:
 *
 *   - The values shown are the kernel's, not the operator's. `appliedLimits`
 *     comes back from the agent after it wrote the cgroup files, and the kernel
 *     rounds and clamps. Echoing the request back would leave somebody
 *     believing a limit that is not in force.
 *   - The usage bars are against each container's own limit, never against the
 *     host. A container at 95% of a 512 MB limit is in trouble on a host with
 *     128 GB free, and a host-relative bar would show it as empty.
 */
export function ResourcesPanel({ container }: { container: Container }) {
  const { t } = useTranslation('ct');
  const live = limitsApplyLive(container);
  const { data: stats } = useContainerStats(container.id, isLive(container));
  const updateLimits = useUpdateLimits();

  // The form starts from what the kernel has, falling back to what was asked
  // for on a container that has never run.
  const inForce = container.appliedLimits ?? container.limits;
  const [draft, setDraft] = useState<Limits>(inForce);

  // Re-seed when the container changes underneath the form — a workflow or
  // another operator can change these while this panel is open, and showing a
  // stale draft as though it were current is the failure this panel is
  // supposed to prevent.
  const [seeded, setSeeded] = useState({
    id: container.id,
    appliedAt: container.limitsAppliedAt,
    applied: container.appliedLimits,
    limits: container.limits,
  });
  if (
    seeded.id !== container.id ||
    seeded.appliedAt !== container.limitsAppliedAt ||
    seeded.applied !== container.appliedLimits ||
    seeded.limits !== container.limits
  ) {
    setSeeded({
      id: container.id,
      appliedAt: container.limitsAppliedAt,
      applied: container.appliedLimits,
      limits: container.limits,
    });
    setDraft(container.appliedLimits ?? container.limits);
  }

  const clamped = updateLimits.data?.clamped ?? [];
  const dirty = JSON.stringify(draft) !== JSON.stringify(inForce);

  return (
    <div className="space-y-6">
      <div className="flex items-start gap-2 rounded-md bg-muted p-3">
        {live ? (
          <IconBolt className="mt-0.5 h-4 w-4 shrink-0 text-primary" aria-hidden="true" />
        ) : (
          <IconInfoCircle className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" aria-hidden="true" />
        )}
        <p className="text-sm text-foreground">{live ? t('resources.liveNote') : t('resources.queuedNote')}</p>
      </div>

      {stats && (
        <section>
          <h3 className="mb-3 font-medium text-foreground">{t('resources.usage.memory')}</h3>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <Meter
              label={t('resources.usage.cpu')}
              value={`${Math.round(stats.cpuPercent)}%`}
              percent={stats.cpuPercent}
            />
            <Meter
              label={t('resources.usage.memory')}
              value={formatBytes(stats.memoryBytes)}
              percent={share(stats.memoryBytes, stats.memoryMaxBytes)}
              caption={
                stats.memoryMaxBytes > 0
                  ? `${percentLabel(stats.memoryBytes, stats.memoryMaxBytes)} ${t('resources.usage.ofLimit')}`
                  : t('resources.usage.noLimit')
              }
              unbounded={stats.memoryMaxBytes <= 0}
            />
            <Meter
              label={t('resources.usage.processes')}
              value={`${stats.pidsCurrent}`}
              percent={share(stats.pidsCurrent, stats.pidsMax)}
              caption={
                stats.pidsMax > 0
                  ? `${percentLabel(stats.pidsCurrent, stats.pidsMax)} ${t('resources.usage.ofLimit')}`
                  : t('resources.usage.noLimit')
              }
              unbounded={stats.pidsMax <= 0}
            />
            <Meter
              label={t('resources.usage.rootfs')}
              value={formatBytes(stats.rootfsUsedBytes)}
              percent={share(stats.rootfsUsedBytes, container.limits.diskQuotaBytes ?? 0)}
              unbounded={!container.limits.diskQuotaBytes}
            />
          </div>

          {/*
            Said out loud rather than left for somebody to discover: McHarbor
            deliberately does not require lxcfs, so `free` and `top` inside the
            container report the host's figures. The numbers above read the
            container's own cgroup and are the correct ones.
          */}
          <p className="mt-3 text-xs text-muted-foreground">{t('resources.lxcfsNote')}</p>
        </section>
      )}

      <section>
        <h3 className="mb-3 font-medium text-foreground">{t('resources.title')}</h3>

        <div className="grid gap-4 sm:grid-cols-2">
          <BytesField
            id="memoryMax"
            label={t('resources.memoryMax')}
            hint={t('resources.memoryMaxHint')}
            bytes={draft.memoryMaxBytes}
            onChange={(bytes) => setDraft({ ...draft, memoryMaxBytes: bytes })}
          />
          <BytesField
            id="memoryHigh"
            label={t('resources.memoryHigh')}
            hint={t('resources.memoryHighHint')}
            bytes={draft.memoryHighBytes}
            onChange={(bytes) => setDraft({ ...draft, memoryHighBytes: bytes })}
          />
          <BytesField
            id="swapMax"
            label={t('resources.swapMax')}
            hint={t('resources.swapMaxHint')}
            bytes={draft.swapMaxBytes}
            onChange={(bytes) => setDraft({ ...draft, swapMaxBytes: bytes ?? 0 })}
          />
          <div>
            <Label htmlFor="pidsMax">{t('resources.pidsMax')}</Label>
            <NumberInput
              id="pidsMax"
              nullable
              min={1}
              value={draft.pidsMax ?? null}
              onChange={(next) => setDraft({ ...draft, pidsMax: next || undefined })}
            />
            <p className="mt-1 text-xs text-muted-foreground">{t('resources.pidsMaxHint')}</p>
          </div>
          <div>
            <Label htmlFor="cpuMax">{t('resources.cpuMax')}</Label>
            <Input
              id="cpuMax"
              value={draft.cpuMax ?? ''}
              placeholder={CPU_UNLIMITED}
              onChange={(ev) => setDraft({ ...draft, cpuMax: ev.target.value || undefined })}
            />
            <p className="mt-1 text-xs text-muted-foreground">{t('resources.cpuMaxHint')}</p>
          </div>
          <div>
            <Label htmlFor="cpuWeight">{t('resources.cpuWeight')}</Label>
            <NumberInput
              id="cpuWeight"
              nullable
              min={1}
              max={10000}
              value={draft.cpuWeight ?? null}
              onChange={(next) => setDraft({ ...draft, cpuWeight: next || undefined })}
            />
            <p className="mt-1 text-xs text-muted-foreground">{t('resources.cpuWeightHint')}</p>
          </div>
        </div>

        <div className="mt-4 flex flex-wrap items-center gap-3">
          <Button
            permission={CT_PERM.MANAGE}
            disabled={!dirty || updateLimits.isPending || !container.hostAvailable}
            onClick={() => updateLimits.mutate({ id: container.id, limits: draft })}
          >
            {t('actions.applyLimits')}
          </Button>
          {dirty && (
            <Button variant="ghost" onClick={() => setDraft(inForce)}>
              {t('actions.cancel')}
            </Button>
          )}
        </div>

        {/*
          The kernel's answer, shown only when it differs from the request.
          Naming the specific fields is the whole of the useful information: a
          memory limit rounded to a page boundary is noise, one clamped to a
          tenth of what was asked is a capacity problem.
        */}
        {clamped.length > 0 && (
          <div className="mt-4 flex items-start gap-2 rounded-md bg-warning/10 p-3">
            <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
            <div>
              <p className="text-sm text-foreground">
                {t('resources.applied.clamped', {
                  count: clamped.length,
                  fields: clamped.join(', '),
                })}
              </p>
              <p className="mt-1 text-xs text-muted-foreground">{t('resources.applied.clampedHint')}</p>
            </div>
          </div>
        )}

        <div className="mt-4">
          <h4 className="mb-1 text-xs font-medium text-muted-foreground">{t('resources.applied.title')}</h4>
          {container.limitsAppliedAt ? (
            <div className="flex flex-wrap gap-2">
              <Badge variant="outline">{formatBytes(inForce.memoryMaxBytes)}</Badge>
              <Badge variant="outline">{inForce.cpuMax || CPU_UNLIMITED}</Badge>
              <Badge variant="outline">{t('resources.applied.pidsBadge', { value: inForce.pidsMax ?? '—' })}</Badge>
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">{t('resources.applied.never')}</p>
          )}
        </div>
      </section>
    </div>
  );
}
