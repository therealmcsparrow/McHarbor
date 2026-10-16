// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { IconHeartbeat } from '@tabler/icons-react';
import { api } from '@core/api/client';
import { PERM } from '@core/auth/permissionKeys';
import { Button } from '@resources/components/ui/Button';
import { useEnvironmentStore } from '@resources/stores/environment';
import { assertSuccess } from '@resources/utils/api-mutation';
import { useContainerHostSelfGate } from '../../hooks/useHostSelfGate';
import { ExecResultCard, type ExecResult } from './ExecResultCard';

type HealthcheckTestButtonProps = {
  containerId: string;
  running: boolean;
  command: string;
  timeoutSeconds: number;
};

/**
 * Runs the healthcheck command once, the way Docker will: the editor's command
 * is a CMD-SHELL test, so it goes through `/bin/sh -c` in the running container.
 * Exit 0 is healthy; anything else, or a run past the timeout, is a failure.
 */
export function HealthcheckTestButton({ containerId, running, command, timeoutSeconds }: HealthcheckTestButtonProps) {
  const { t } = useTranslation('containers');
  const gate = useContainerHostSelfGate(containerId);
  const envId = useEnvironmentStore((s) => s.currentId);
  const cmd = command.trim();

  const test = useMutation({
    mutationFn: async () => {
      const params = envId ? `?env=${envId}` : '';
      return assertSuccess(
        await api.post<ExecResult>(`/containers/${containerId}/exec${params}`, {
          command: [cmd],
          shell: '/bin/sh',
        }),
      );
    },
  });

  const result = test.data;
  const tooSlow = result !== undefined && timeoutSeconds > 0 && result.durationMs > timeoutSeconds * 1000;
  const healthy = result !== undefined && result.exitCode === 0 && !tooSlow;

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center gap-3">
        <Button
          size="sm"
          variant="outline"
          permission={gate.exec(PERM.CONTAINERS_MANAGE)}
          disabled={!running || !cmd || test.isPending}
          onClick={() => test.mutate()}
        >
          <IconHeartbeat className="size-4" aria-hidden="true" />
          {test.isPending ? t('environment.healthTesting') : t('environment.healthTest')}
        </Button>
        {!running && <span className="text-xs text-muted-foreground">{t('environment.healthTestNotRunning')}</span>}
      </div>
      {result && (
        <output className={healthy ? 'block text-xs text-success' : 'block text-xs text-destructive'}>
          {result.exitCode === 0
            ? t('environment.healthTestHealthy', { ms: result.durationMs })
            : t('environment.healthTestUnhealthy', { code: result.exitCode })}
          {tooSlow && ` ${t('environment.healthTestTooSlow', { ms: result.durationMs, timeout: timeoutSeconds })}`}
        </output>
      )}
      {result && <ExecResultCard lastResult={result} />}
    </div>
  );
}
