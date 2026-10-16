// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describeDestructiveDialog } from '../../../test/destructive';
import type { HostTarget } from '@resources/hooks/useHostUpdates';
import { HostDaemonReloadDialog } from '@resources/components/host/HostDaemonReloadDialog';
import {
  HostServiceActionDialog,
  type HostServiceConfirmAction,
} from '@resources/components/host/HostServiceActionDialog';
import { HostServiceRevertDialog } from '@resources/components/host/HostServiceRevertDialog';

const confirmLabels: Record<HostServiceConfirmAction, string> = {
  stop: 'Stop',
  restart: 'Restart',
  disable: 'Disable',
};

type HostCase = { label: string; host: HostTarget; base: string; manage: string };

const ENV_HOST: HostCase = {
  label: 'environment host',
  host: { kind: 'environment', envId: 'env-1' },
  base: '*/api/host-services',
  manage: 'host_services.manage',
};

const SELF_HOST: HostCase = {
  label: "McHarbor's own host",
  host: { kind: 'self' },
  base: '*/api/host-services/self',
  manage: 'host_self.manage',
};

type RenderProps = {
  onOpenChange: (open: boolean) => void;
  onSuccess: () => void;
};

function actionCase({ label, host, base, manage }: HostCase, action: HostServiceConfirmAction) {
  return {
    name: `HostServiceActionDialog ${action} on the ${label}`,
    method: 'post' as const,
    path: `${base}/nginx.service/${action}`,
    permission: manage,
    confirmLabel: confirmLabels[action],
    body: {},
    render: ({ onOpenChange, onSuccess }: RenderProps) => (
      <HostServiceActionDialog
        host={host}
        target={{ unit: 'nginx.service', action }}
        onOpenChange={onOpenChange}
        onSuccess={onSuccess}
      />
    ),
  };
}

function revertCase({ label, host, base, manage }: HostCase) {
  return {
    name: `HostServiceRevertDialog on the ${label}`,
    method: 'delete' as const,
    path: `${base}/nginx.service/override`,
    permission: manage,
    confirmLabel: 'Revert',
    render: ({ onOpenChange, onSuccess }: RenderProps) => (
      <HostServiceRevertDialog host={host} unit="nginx.service" onOpenChange={onOpenChange} onSuccess={onSuccess} />
    ),
  };
}

function reloadCase({ label, host, base, manage }: HostCase) {
  return {
    name: `HostDaemonReloadDialog on the ${label}`,
    method: 'post' as const,
    path: `${base}/daemon-reload`,
    permission: manage,
    confirmLabel: 'Reload systemd',
    body: {},
    render: ({ onOpenChange, onSuccess }: RenderProps) => (
      <HostDaemonReloadDialog host={host} open onOpenChange={onOpenChange} onSuccess={onSuccess} />
    ),
  };
}

describeDestructiveDialog({ ...actionCase(ENV_HOST, 'stop') });
describeDestructiveDialog({ ...actionCase(ENV_HOST, 'restart') });
describeDestructiveDialog({ ...actionCase(ENV_HOST, 'disable') });
describeDestructiveDialog({ ...revertCase(ENV_HOST) });
describeDestructiveDialog({ ...reloadCase(ENV_HOST) });

describeDestructiveDialog({ ...actionCase(SELF_HOST, 'stop') });
describeDestructiveDialog({ ...actionCase(SELF_HOST, 'restart') });
describeDestructiveDialog({ ...actionCase(SELF_HOST, 'disable') });
describeDestructiveDialog({ ...revertCase(SELF_HOST) });
describeDestructiveDialog({ ...reloadCase(SELF_HOST) });
