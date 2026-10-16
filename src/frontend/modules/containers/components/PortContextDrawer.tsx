// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCircleCheck, IconCircleX } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { InfoRow } from '@resources/components/ui/InfoRow';
import { Spinner } from '@resources/components/ui/Spinner';
import type { PortMappingEntry } from '../types/edit-form';
import { ContextDrawer } from './ContextDrawer';
type TestStepId = 'url' | 'mapping' | 'request';
type TestStepStatus = 'pending' | 'running' | 'success' | 'error';

type TestStep = { id: TestStepId; status: TestStepStatus };

type PortContextDrawerProps = {
  mapping: PortMappingEntry | null;
  allowConnectionTest: boolean;
  onClose: () => void;
};

export function PortContextDrawer({ mapping, allowConnectionTest, onClose }: PortContextDrawerProps) {
  const { t } = useTranslation('containers');
  const [steps, setSteps] = useState<TestStep[]>(() => initialTestSteps());
  const [summary, setSummary] = useState<string | null>(null);
  const testHost =
    mapping?.hostIp && !['0.0.0.0', '::', ''].includes(mapping.hostIp)
      ? mapping.hostIp
      : typeof window !== 'undefined'
        ? window.location.hostname
        : '';
  const testUrl =
    allowConnectionTest && mapping?.protocol === 'tcp' && mapping.hostPort && testHost
      ? `http://${testHost.includes(':') ? `[${testHost}]` : testHost}:${mapping.hostPort}`
      : null;
  const testRunning = steps.some((step) => step.status === 'running');

  const [appliedTestUrl, setAppliedTestUrl] = useState(testUrl);
  if (appliedTestUrl !== testUrl) {
    setAppliedTestUrl(testUrl);
    setSteps(initialTestSteps());
    setSummary(null);
  }

  function updateStep(id: TestStepId, statusCode: TestStepStatus) {
    setSteps((current) => current.map((step) => (step.id === id ? { ...step, status: statusCode } : step)));
  }

  async function runConnectionTest() {
    if (!testUrl) return;

    setSummary(null);
    setSteps(initialTestSteps());
    updateStep('url', 'running');
    await delay(150);
    updateStep('url', 'success');

    updateStep('mapping', 'running');
    await delay(150);
    updateStep('mapping', 'success');

    updateStep('request', 'running');
    const controller = new AbortController();
    const timeout = window.setTimeout(() => controller.abort(), 8000);
    try {
      await fetch(testUrl, {
        cache: 'no-store',
        mode: 'no-cors',
        signal: controller.signal,
      });
      updateStep('request', 'success');
      setSummary(t('networkTab.testSummarySuccess', { url: testUrl }));
    } catch (error) {
      updateStep('request', 'error');
      setSummary(
        error instanceof DOMException && error.name === 'AbortError'
          ? t('networkTab.testFailureTimeout', { url: testUrl })
          : t('networkTab.testSummaryFailure', { url: testUrl }),
      );
    } finally {
      window.clearTimeout(timeout);
    }
  }

  return (
    <ContextDrawer
      open={mapping !== null}
      onOpenChange={(isOpen) => !isOpen && onClose()}
      title={t('networkTab.portDetailsTitle')}
      description={t('networkTab.portDetailsDescription')}
      footer={
        testUrl ? (
          <Button onClick={runConnectionTest} disabled={testRunning}>
            {testRunning ? <Spinner size="sm" /> : null}
            {testRunning
              ? t('networkTab.testConnectionRunning')
              : summary
                ? t('networkTab.testConnectionAgain')
                : t('networkTab.testConnection')}
          </Button>
        ) : undefined
      }
    >
      {mapping ? (
        <div className="space-y-4">
          <div className="space-y-1">
            <InfoRow label={t('networkTab.hostIp')}>
              <span className="font-mono text-xs">{mapping.hostIp}</span>
            </InfoRow>
            <InfoRow label={t('networkTab.hostPort')}>
              <span className="font-mono text-xs">{mapping.hostPort}</span>
            </InfoRow>
            <InfoRow label={t('networkTab.containerPort')}>
              <span className="font-mono text-xs">{mapping.containerPort}</span>
            </InfoRow>
            <InfoRow label={t('networkTab.protocol')}>{mapping.protocol.toUpperCase()}</InfoRow>
          </div>
          {testUrl ? (
            <div className="rounded-lg border border-border bg-muted/30 p-3">
              <div className="mb-3 space-y-1">
                <div className="text-xs font-semibold tracking-wide text-muted-foreground uppercase">
                  {t('networkTab.testUrlLabel')}
                </div>
                <div className="rounded-md bg-background px-2 py-1 font-mono text-xs break-all text-foreground">
                  {testUrl}
                </div>
              </div>
              <div className="space-y-2">
                {steps.map((step) => (
                  <div key={step.id} className="mc-glass flex gap-3 rounded-md p-3">
                    <StepStatusIcon status={step.status} />
                    <div className="min-w-0 space-y-1">
                      <div className="text-sm font-medium text-foreground">
                        {t(`networkTab.testStep.${step.id}.title`)}
                      </div>
                      <div className="text-xs text-muted-foreground">
                        {t(`networkTab.testStep.${step.id}.description`, {
                          host: testHost,
                          port: mapping.hostPort,
                          url: testUrl,
                        })}
                      </div>
                    </div>
                  </div>
                ))}
              </div>
              {summary ? (
                <div className="mt-3 rounded-md border border-border bg-background p-3 text-xs text-muted-foreground">
                  {summary}
                </div>
              ) : null}
            </div>
          ) : null}
        </div>
      ) : null}
    </ContextDrawer>
  );
}

function initialTestSteps(): TestStep[] {
  return [
    { id: 'url', status: 'pending' },
    { id: 'mapping', status: 'pending' },
    { id: 'request', status: 'pending' },
  ];
}

function delay(ms: number) {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}

function StepStatusIcon({ status: statusValue }: { status: TestStepStatus }) {
  if (statusValue === 'running') {
    return <Spinner size="sm" className="mt-0.5 shrink-0" />;
  }
  if (statusValue === 'success') {
    return <IconCircleCheck className="mt-0.5 size-4 shrink-0 text-chart-1" />;
  }
  if (statusValue === 'error') {
    return <IconCircleX className="mt-0.5 size-4 shrink-0 text-danger" />;
  }
  return <span className="mt-1 size-3 shrink-0 rounded-full border border-border" />;
}
