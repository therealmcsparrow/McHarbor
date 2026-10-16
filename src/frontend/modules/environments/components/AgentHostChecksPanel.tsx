// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  IconAlertCircle,
  IconAlertTriangle,
  IconBrandDocker,
  IconCheck,
  IconDeviceDesktopAnalytics,
} from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { PERM } from '@core/auth/permissionKeys';
import { useProbePrerequisites, type DeployRequest, type PrerequisiteReport } from '../hooks/useEnvironmentActions';

export type HostCheckScope = 'docker' | 'vm';

export interface HostCheckState {
  report?: PrerequisiteReport;
  error?: string;
}

interface AgentHostChecksPanelProps {
  canProbe: boolean;
  buildRequest: () => DeployRequest;
  results: Partial<Record<HostCheckScope, HostCheckState>>;
  onResult: (scope: HostCheckScope, state: HostCheckState) => void;
}

export function AgentHostChecksPanel({ canProbe, buildRequest, results, onResult }: AgentHostChecksPanelProps) {
  const { t } = useTranslation('environments');
  const dockerProbe = useProbePrerequisites();
  const vmProbe = useProbePrerequisites();

  const probes: Record<HostCheckScope, ReturnType<typeof useProbePrerequisites>> = {
    docker: dockerProbe,
    vm: vmProbe,
  };

  const run = (scope: HostCheckScope) => {
    onResult(scope, {});
    probes[scope].mutate(
      { ...buildRequest(), scope },
      {
        onSuccess: (result) => {
          if (!result.success || !result.prerequisites) {
            onResult(scope, {
              error: result.error ?? t('create.hostChecks.failed'),
            });
            return;
          }
          onResult(scope, { report: result.prerequisites });
        },
        onError: (err: Error) => {
          onResult(scope, {
            error: err.message || t('create.hostChecks.failed'),
          });
        },
      },
    );
  };

  return (
    <div className="rounded-lg border border-border p-3">
      <p className="text-sm font-medium text-foreground">{t('create.hostChecks.title')}</p>
      <p className="mt-0.5 text-xs text-muted-foreground">{t('create.hostChecks.body')}</p>

      <div className="mt-3 flex flex-wrap gap-2">
        <Button
          size="sm"
          variant="outline"
          permission={PERM.ENVIRONMENTS_MANAGE}
          disabled={!canProbe || dockerProbe.isPending || vmProbe.isPending}
          onClick={() => run('docker')}
        >
          {dockerProbe.isPending ? (
            <Spinner className="mr-1.5 h-4 w-4" />
          ) : (
            <IconBrandDocker className="mr-1.5 h-4 w-4" aria-hidden="true" />
          )}
          {t('create.hostChecks.docker')}
        </Button>
        <Button
          size="sm"
          variant="outline"
          permission={PERM.ENVIRONMENTS_MANAGE}
          disabled={!canProbe || dockerProbe.isPending || vmProbe.isPending}
          onClick={() => run('vm')}
        >
          {vmProbe.isPending ? (
            <Spinner className="mr-1.5 h-4 w-4" />
          ) : (
            <IconDeviceDesktopAnalytics className="mr-1.5 h-4 w-4" aria-hidden="true" />
          )}
          {t('create.hostChecks.vm')}
        </Button>
      </div>

      {!canProbe ? <p className="mt-2 text-xs text-muted-foreground">{t('create.hostChecks.hint')}</p> : null}

      <HostCheckResult scope="docker" state={results.docker} />
      <HostCheckResult scope="vm" state={results.vm} />
    </div>
  );
}

function HostCheckResult({ scope, state }: { scope: HostCheckScope; state?: HostCheckState }) {
  const { t } = useTranslation('environments');

  if (!state || (!state.report && !state.error)) return null;

  if (state.error) {
    return (
      <div className="mt-3 flex items-start gap-2 rounded-md border border-destructive/40 bg-destructive/10 p-2.5">
        <IconAlertCircle className="mt-0.5 h-4 w-4 shrink-0 text-destructive" aria-hidden="true" />
        <div className="min-w-0 text-xs">
          <p className="font-medium text-destructive">{t(`create.hostChecks.${scope}`)}</p>
          <p className="mt-0.5 break-words whitespace-pre-wrap text-muted-foreground">{state.error}</p>
        </div>
      </div>
    );
  }

  const report = state.report;
  if (!report) return null;

  const blocking = report.items.filter((item) => item.required && !item.present);
  const ready = blocking.length === 0;

  return (
    <div className="mt-3 rounded-md border border-border p-2.5">
      <div className="flex items-center gap-2">
        {ready ? (
          <IconCheck className="h-4 w-4 shrink-0 text-success" aria-hidden="true" />
        ) : (
          <IconAlertTriangle className="h-4 w-4 shrink-0 text-destructive" aria-hidden="true" />
        )}
        <p className="text-xs font-medium text-foreground">
          {ready ? t(`create.hostChecks.${scope}Ready`) : t(`create.hostChecks.${scope}NotReady`)}
        </p>
      </div>

      <p className="mt-1 text-xs text-muted-foreground">
        {t('create.prereq.detected', {
          os: report.os || '—',
          arch: report.arch || '—',
          distro: report.distro || '—',
        })}
      </p>

      <ul className="mt-2 space-y-1.5">
        {report.items.map((item) => (
          <li key={item.key} className="flex items-start gap-2 text-xs">
            {item.present ? (
              <IconCheck className="mt-0.5 h-3.5 w-3.5 shrink-0 text-success" aria-hidden="true" />
            ) : (
              <IconAlertTriangle
                className={`mt-0.5 h-3.5 w-3.5 shrink-0 ${item.required ? 'text-destructive' : 'text-warning'}`}
                aria-hidden="true"
              />
            )}
            <span className="min-w-0 flex-1">
              <span className="text-foreground">{item.label}</span>
              {!item.present && item.required ? (
                <span className="ml-1 text-destructive">{t('create.hostChecks.blocks')}</span>
              ) : null}
              {item.detail ? <span className="block text-muted-foreground">{item.detail}</span> : null}
              {!item.present && item.remediation ? (
                <code className="mt-0.5 block rounded bg-muted px-1.5 py-1 font-mono text-[11px] break-all text-muted-foreground">
                  {item.remediation}
                </code>
              ) : null}
            </span>
          </li>
        ))}
      </ul>

      {scope === 'vm' ? <p className="mt-2 text-xs text-muted-foreground">{t('create.hostChecks.vmNote')}</p> : null}
    </div>
  );
}
