// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconArrowRight, IconPlugConnected, IconRoute } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { Spinner } from '@resources/components/ui/Spinner';
import { cn } from '@resources/utils/cn';
import { useAgents, useDirectTransferTest } from '../hooks/useAgentSettings';
import type { AgentInfo, DirectTransferTestResult } from '../types';

function agentLabel(agent: AgentInfo) {
  return agent.hostname ? `${agent.envName} (${agent.hostname})` : agent.envName;
}

function ResultRow({
  label,
  value,
  mono,
}: {
  label: string;
  value?: string | number | boolean;
  mono?: boolean;
}) {
  if (value === undefined || value === '') return null;
  return (
    <div className="grid gap-1 rounded-lg border border-border/60 bg-background/40 px-3 py-2 sm:grid-cols-[10rem_1fr]">
      <span className="text-xs font-medium text-muted-foreground">{label}</span>
      <span className={cn('break-all text-sm text-foreground', mono && 'font-mono text-xs')}>
        {String(value)}
      </span>
    </div>
  );
}

export function AgentDirectTransferTest() {
  const { t } = useTranslation('settings');
  const { data: agents = [], isLoading } = useAgents();
  const transferTest = useDirectTransferTest();
  const [sourceEnvId, setSourceEnvId] = useState('');
  const [targetEnvId, setTargetEnvId] = useState('');
  const [result, setResult] = useState<DirectTransferTestResult | null>(null);

  const connectedAgents = useMemo(
    () => agents.filter((agent) => agent.status === 'connected'),
    [agents]
  );

  const sourceOptions = useMemo(
    () => connectedAgents.map((agent) => ({ value: agent.envId, label: agentLabel(agent) })),
    [connectedAgents]
  );

  const targetOptions = useMemo(
    () =>
      connectedAgents
        .filter((agent) => agent.envId !== sourceEnvId)
        .map((agent) => ({ value: agent.envId, label: agentLabel(agent) })),
    [connectedAgents, sourceEnvId]
  );

  useEffect(() => {
    const firstSource = sourceOptions[0];
    if (!sourceEnvId && firstSource) {
      setSourceEnvId(firstSource.value);
    }
  }, [sourceEnvId, sourceOptions]);

  useEffect(() => {
    if (targetOptions.length === 0) {
      setTargetEnvId('');
      return;
    }
    const firstTarget = targetOptions[0];
    if (firstTarget && (!targetEnvId || !targetOptions.some((option) => option.value === targetEnvId))) {
      setTargetEnvId(firstTarget.value);
    }
  }, [targetEnvId, targetOptions]);

  const canRun = Boolean(sourceEnvId && targetEnvId && sourceEnvId !== targetEnvId);

  const handleRun = () => {
    if (!canRun) return;
    transferTest.mutate(
      { sourceEnvId, targetEnvId },
      {
        onSuccess: setResult,
      }
    );
  };

  const phaseLabel =
    result?.phase && t(`agent.directTransferTest.phases.${result.phase}`, result.phase);

  return (
    <div className="space-y-4 rounded-lg border border-border p-4">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-2">
            <IconRoute className="size-5 text-primary" />
            <h3 className="text-base font-semibold text-foreground">
              {t('agent.directTransferTest.title')}
            </h3>
          </div>
          <p className="mt-1 text-sm text-muted-foreground">
            {t('agent.directTransferTest.description')}
          </p>
        </div>
        <Badge variant={connectedAgents.length >= 2 ? 'success' : 'secondary'}>
          {t('agent.directTransferTest.connectedCount', { count: connectedAgents.length })}
        </Badge>
      </div>

      {isLoading ? (
        <div className="flex h-24 items-center justify-center">
          <Spinner />
        </div>
      ) : connectedAgents.length < 2 ? (
        <div className="rounded-lg border border-dashed border-border px-4 py-6 text-sm text-muted-foreground">
          {t('agent.directTransferTest.notEnoughAgents')}
        </div>
      ) : (
        <>
          <div className="grid gap-3 md:grid-cols-[1fr_auto_1fr] md:items-end">
            <div>
              <Label className="mb-2">{t('agent.directTransferTest.sourceAgent')}</Label>
              <Select
                value={sourceEnvId}
                onChange={setSourceEnvId}
                options={sourceOptions}
                placeholder={t('agent.directTransferTest.selectSourceAgent')}
                ariaLabel={t('agent.directTransferTest.sourceAgent')}
              />
            </div>

            <div className="hidden justify-center pb-3 md:flex">
              <IconArrowRight className="size-5 text-muted-foreground" />
            </div>

            <div>
              <Label className="mb-2">{t('agent.directTransferTest.targetAgent')}</Label>
              <Select
                value={targetEnvId}
                onChange={setTargetEnvId}
                options={targetOptions}
                placeholder={t('agent.directTransferTest.selectTargetAgent')}
                ariaLabel={t('agent.directTransferTest.targetAgent')}
              />
            </div>
          </div>

          <Button onClick={handleRun} disabled={!canRun || transferTest.isPending}>
            {transferTest.isPending ? (
              <>
                <Spinner className="mr-2 size-4" />
                {t('agent.directTransferTest.testing')}
              </>
            ) : (
              <>
                <IconPlugConnected className="mr-2 size-4" />
                {t('agent.directTransferTest.run')}
              </>
            )}
          </Button>
        </>
      )}

      {result && (
        <div className="space-y-3 rounded-lg border border-border/70 bg-muted/20 p-3">
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant={result.success ? 'success' : 'destructive'}>
              {result.success
                ? t('agent.directTransferTest.success')
                : t('agent.directTransferTest.failed')}
            </Badge>
            {phaseLabel && <Badge variant="outline">{phaseLabel}</Badge>}
          </div>

          <div className="grid gap-2">
            <ResultRow label={t('agent.directTransferTest.source')} value={result.sourceName} />
            <ResultRow
              label={t('agent.directTransferTest.sourceVersion')}
              value={result.sourceVersion}
            />
            <ResultRow label={t('agent.directTransferTest.target')} value={result.targetName} />
            <ResultRow
              label={t('agent.directTransferTest.targetVersion')}
              value={result.targetVersion}
            />
            <ResultRow
              label={t('agent.directTransferTest.statusCode')}
              value={result.statusCode}
            />
            <ResultRow
              label={t('agent.directTransferTest.duration')}
              value={t('agent.directTransferTest.durationValue', { duration: result.durationMs })}
            />
            <ResultRow
              label={t('agent.directTransferTest.targetTransferUrl')}
              value={result.targetTransferUrl}
              mono
            />
            <ResultRow
              label={t('agent.directTransferTest.probeUrl')}
              value={result.probeUrl}
              mono
            />
            <ResultRow label={t('agent.directTransferTest.error')} value={result.error} />
          </div>
        </div>
      )}
    </div>
  );
}
