// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconSearch, IconCheck, IconAlertTriangle } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { PERM } from '@core/auth/permissionKeys';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Spinner } from '@resources/components/ui/Spinner';
import type { PrerequisiteReport } from '../hooks/useEnvironmentActions';
import { evaluatePrereqGate } from '../hooks/prereqGate';
import { PrerequisiteInstallForm } from './PrerequisiteInstallForm';
import { PrerequisiteItemRow } from './PrerequisiteItemRow';

interface AgentPrerequisitesPanelProps {
  report?: PrerequisiteReport;
  error?: string;
  output?: string;
  probing: boolean;
  installing: boolean;
  canProbe: boolean;
  rootPassword: string;
  onRootPasswordChange: (value: string) => void;
  selected: string[];
  onProbe: () => void;
  onInstall: () => void;
  onSelectedChange: (keys: string[]) => void;
}

export function AgentPrerequisitesPanel({
  report,
  error,
  output,
  probing,
  installing,
  canProbe,
  rootPassword,
  onRootPasswordChange,
  selected,
  onProbe,
  onInstall,
  onSelectedChange,
}: AgentPrerequisitesPanelProps) {
  const { t } = useTranslation('environments');

  const toggle = (key: string, on: boolean) => {
    onSelectedChange(on ? [...selected, key] : selected.filter((entry) => entry !== key));
  };

  const gate = evaluatePrereqGate(report, selected, rootPassword);

  const installable = (report?.items ?? []).filter((item) => !item.present && Boolean(item.remediation));

  return (
    <div className="rounded-lg border border-border p-3">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-sm font-medium text-foreground">{t('create.prereq.title')}</p>
          <p className="mt-0.5 text-xs text-muted-foreground">{t('create.prereq.body')}</p>
        </div>
        <Button
          size="sm"
          variant="outline"
          permission={PERM.ENVIRONMENTS_MANAGE}
          disabled={!canProbe || probing || installing}
          onClick={onProbe}
        >
          {probing ? (
            <Spinner className="mr-1.5 h-4 w-4" />
          ) : (
            <IconSearch className="mr-1.5 h-4 w-4" aria-hidden="true" />
          )}
          {t('create.prereq.check')}
        </Button>
      </div>

      {error ? (
        <ErrorState variant="banner" className="mt-3" title={t('create.prereq.failed')} description={error} />
      ) : null}

      {report ? (
        <div className="mt-3 space-y-2">
          <p className="text-xs text-muted-foreground">
            {t('create.prereq.detected', {
              os: report.os || '—',
              arch: report.arch || '—',
              distro: report.distro || '—',
            })}
          </p>

          <p className="text-xs text-muted-foreground">
            {t('create.prereq.escalation.label')}{' '}
            <span className={report.escalation === 'unavailable' ? 'text-destructive' : 'text-foreground'}>
              {t(`create.prereq.escalation.${report.escalation ?? 'unavailable'}`)}
            </span>
          </p>

          {report.needsRootPassword ? (
            <div className="rounded-md border border-border bg-muted/40 p-2.5">
              <Label htmlFor="prereq-root-password" className="mb-1.5 text-xs">
                {t('create.prereq.rootPassword')}
              </Label>
              <Input
                id="prereq-root-password"
                type="password"
                value={rootPassword}
                onChange={(e) => onRootPasswordChange(e.target.value)}
                autoComplete="off"
              />
              <p className="mt-1 text-xs text-muted-foreground">{t('create.prereq.rootPasswordHelp')}</p>
            </div>
          ) : null}

          <output className={`flex items-center gap-1.5 text-xs ${gate.ready ? 'text-success' : 'text-destructive'}`}>
            {gate.ready ? (
              <IconCheck className="h-3.5 w-3.5" aria-hidden="true" />
            ) : (
              <IconAlertTriangle className="h-3.5 w-3.5" aria-hidden="true" />
            )}
            {gate.ready
              ? t('create.prereq.ready')
              : gate.noEscalation
                ? t('create.prereq.blockedEscalation')
                : t('create.prereq.blockedMissing')}
          </output>

          <ul className="space-y-1.5">
            {report.items.map((item) => (
              <PrerequisiteItemRow key={item.key} item={item} />
            ))}
          </ul>

          {installable.length > 0 ? (
            <PrerequisiteInstallForm
              installable={installable}
              selected={selected}
              installing={installing}
              disabled={!canProbe || installing || probing}
              onToggle={toggle}
              onInstall={onInstall}
            />
          ) : null}
        </div>
      ) : error ? null : (
        <p className="mt-2 text-xs text-muted-foreground">{t('create.prereq.notChecked')}</p>
      )}

      {output ? (
        <div className="mt-3 border-t border-border pt-3">
          <p className="text-xs font-medium text-foreground">{t('create.prereq.outputTitle')}</p>
          <pre className="mt-1.5 max-h-48 overflow-auto rounded bg-muted p-2 font-mono text-[11px] break-words whitespace-pre-wrap text-muted-foreground">
            {output}
          </pre>
        </div>
      ) : null}
    </div>
  );
}
