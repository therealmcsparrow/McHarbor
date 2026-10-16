// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IconBox,
  IconBrandDocker,
  IconHexagons,
  IconCloudComputing,
  IconDeviceDesktop,
  IconServer2,
} from '@tabler/icons-react';
import { Spinner } from '@resources/components/ui/Spinner';
import type { HostRebootInventory } from '@resources/hooks/useHostReboot';

type HostRebootDisclaimersProps = {
  inventory?: HostRebootInventory;
  loading: boolean;
  self: boolean;
};

type DisclaimerProps = {
  icon: ReactNode;
  title: string;
  body: string;
  detected?: string;
  warning?: string;
};

function Disclaimer({ icon, title, body, detected, warning }: DisclaimerProps) {
  return (
    <li className="flex gap-3">
      <span className="mt-0.5 shrink-0 text-muted-foreground">{icon}</span>
      <div className="min-w-0 space-y-0.5">
        <p className="text-sm font-medium text-foreground">{title}</p>
        <p className="text-xs text-muted-foreground">{body}</p>
        {detected ? <p className="text-xs text-foreground">{detected}</p> : null}
        {warning ? <p className="text-xs text-warning">{warning}</p> : null}
      </div>
    </li>
  );
}

export function HostRebootDisclaimers({ inventory, loading, self: isSelf }: HostRebootDisclaimersProps) {
  const { t } = useTranslation('environments');
  const inv = inventory;
  const iconClass = 'size-4';

  const containersDetected = !inv?.containersKnown
    ? t('detail.hostReboot.disclaimer.unknown')
    : inv.containers === 0
      ? t('detail.hostReboot.disclaimer.notDetected')
      : t('detail.hostReboot.disclaimer.containersDetected', {
          total: inv.containers,
          returning: inv.containersReturning,
        });
  const guestsDetected = !inv?.guestsKnown
    ? t('detail.hostReboot.disclaimer.unknown')
    : inv.guests === 0
      ? t('detail.hostReboot.disclaimer.notDetected')
      : t('detail.hostReboot.disclaimer.guestsDetected', { guests: inv.guests });
  const blockers = inv?.guestBlockers?.length
    ? t('detail.hostReboot.disclaimer.guestBlockers', { names: inv.guestBlockers.join(', ') })
    : undefined;
  const kubernetes = inv?.kubernetes?.length
    ? undefined
    : inv?.kubernetesKnown
      ? t('detail.hostReboot.disclaimer.notDetected')
      : t('detail.hostReboot.disclaimer.unknown');
  const kubernetesWarning = inv?.kubernetes?.length
    ? t('detail.hostReboot.disclaimer.kubernetesDetected', { names: inv.kubernetes.join(', ') })
    : undefined;
  const swarmDetected =
    inv?.swarm === 'manager'
      ? t('detail.hostReboot.disclaimer.swarmManager')
      : inv?.swarm === 'worker'
        ? undefined
        : inv
          ? t('detail.hostReboot.disclaimer.notDetected')
          : t('detail.hostReboot.disclaimer.unknown');
  const swarmWarning =
    inv?.swarm === 'worker' ? t('detail.hostReboot.disclaimer.swarmWorker', { tasks: inv.swarmTasks }) : undefined;

  return (
    <div className="space-y-3">
      <div className="flex items-center gap-2">
        <p className="text-sm font-medium text-foreground">{t('detail.hostReboot.dialog.disclaimersTitle')}</p>
        {loading ? (
          <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <Spinner className="size-3" />
            {t('detail.hostReboot.dialog.detecting')}
          </span>
        ) : null}
      </div>
      <ul className="max-h-[40vh] space-y-3 overflow-auto rounded-md border border-border p-3">
        <Disclaimer
          icon={<IconBrandDocker className={iconClass} aria-hidden="true" />}
          title={t('detail.hostReboot.disclaimer.containersTitle')}
          body={t('detail.hostReboot.disclaimer.containers')}
          detected={loading ? undefined : containersDetected}
        />
        <Disclaimer
          icon={<IconDeviceDesktop className={iconClass} aria-hidden="true" />}
          title={t('detail.hostReboot.disclaimer.vmsTitle')}
          body={t('detail.hostReboot.disclaimer.vms')}
          detected={loading ? undefined : guestsDetected}
          warning={blockers}
        />
        <Disclaimer
          icon={<IconBox className={iconClass} aria-hidden="true" />}
          title={t('detail.hostReboot.disclaimer.lxcTitle')}
          body={t('detail.hostReboot.disclaimer.lxc')}
        />
        <Disclaimer
          icon={<IconHexagons className={iconClass} aria-hidden="true" />}
          title={t('detail.hostReboot.disclaimer.kubernetesTitle')}
          body={t('detail.hostReboot.disclaimer.kubernetes')}
          detected={loading ? undefined : kubernetes}
          warning={kubernetesWarning}
        />
        <Disclaimer
          icon={<IconCloudComputing className={iconClass} aria-hidden="true" />}
          title={t('detail.hostReboot.disclaimer.swarmTitle')}
          body={t('detail.hostReboot.disclaimer.swarm')}
          detected={loading ? undefined : swarmDetected}
          warning={swarmWarning}
        />
        {isSelf || inv?.mcharborHost ? (
          <Disclaimer
            icon={<IconServer2 className={iconClass} aria-hidden="true" />}
            title={t('detail.hostReboot.disclaimer.mcharborTitle')}
            body={t('detail.hostReboot.disclaimer.mcharbor')}
          />
        ) : null}
      </ul>
      <p className="text-xs text-muted-foreground">{t('detail.hostReboot.disclaimer.refused')}</p>
    </div>
  );
}
