// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconDownload, IconShieldCheck, IconPuzzle } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { PERM } from '@core/auth/permissionKeys';
import { Spinner } from '@resources/components/ui/Spinner';
import { ProgressBar } from '@resources/components/ui/ProgressBar';
import { formatDate } from '@resources/utils/format';
import { useAuditCoverage, useExportAuditCoverage, usePluginAuditSummary } from '@resources/hooks/useAuditLogs';

/**
 * How much of the audited route surface actually maps to an audit action, and
 * which plugins are writing audit events.
 *
 * Both endpoints have existed and returned real data all along; the hooks were
 * written and never rendered, which is the only reason they read as orphaned.
 * The question the panel answers — "is the audit log actually covering the
 * things it is supposed to cover" — is one an operator cannot answer from a
 * list of events, however long.
 *
 * Unmapped-required is the number that matters, so it is the one that turns
 * the badge: a route the catalog says must be audited and that maps to no
 * action is a hole in the record, not a statistic.
 */
export function AuditCoveragePanel() {
  const { t } = useTranslation('common');
  const { data: coverage, isLoading } = useAuditCoverage();
  const { data: plugins } = usePluginAuditSummary();
  const exportCoverage = useExportAuditCoverage();

  if (isLoading) {
    return (
      <div className="mc-glass flex items-center gap-2 rounded-lg p-6 text-sm text-muted-foreground">
        <Spinner size="sm" />
        {t('audit.coverage.loading')}
      </div>
    );
  }
  if (!coverage) return null;

  const percent = Math.round(coverage.coveragePercent);
  const hasHoles = coverage.unmappedRequired > 0;

  return (
    <div className="mc-glass space-y-4 rounded-lg p-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 className="flex items-center gap-2 text-sm font-medium text-foreground">
            <IconShieldCheck className="size-4 text-muted-foreground" aria-hidden="true" />
            {t('audit.coverage.title')}
          </h3>
          <p className="mt-1 text-xs text-muted-foreground">{t('audit.coverage.description')}</p>
        </div>
        <div className="flex items-center gap-2">
          <Badge variant={hasHoles ? 'warning' : 'success'}>{t('audit.coverage.percent', { percent })}</Badge>
          <Button
            permission={PERM.AUDIT_EXPORT}
            variant="outline"
            size="sm"
            onClick={() => exportCoverage.mutate()}
            disabled={exportCoverage.isPending}
          >
            <IconDownload className="size-3.5" aria-hidden="true" />
            {t('audit.coverage.export')}
          </Button>
        </div>
      </div>

      <ProgressBar value={percent} />

      <div className="grid gap-3 sm:grid-cols-3">
        <Stat label={t('audit.coverage.required')} value={coverage.requiredRoutes} />
        <Stat label={t('audit.coverage.mapped')} value={coverage.mappedRoutes} />
        <Stat
          label={t('audit.coverage.unmapped')}
          value={coverage.unmappedRequired}
          tone={hasHoles ? 'warning' : undefined}
        />
      </div>

      {/* Plugins write into the same audit log as the host, so a coverage
          answer that ignored them would be incomplete. Only rendered when a
          plugin has actually emitted something. */}
      {plugins && plugins.length > 0 && (
        <div className="space-y-2">
          <p className="flex items-center gap-2 text-xs font-medium tracking-wider text-muted-foreground uppercase">
            <IconPuzzle className="size-3.5" aria-hidden="true" />
            {t('audit.coverage.pluginsTitle')}
          </p>
          <ul className="space-y-1">
            {plugins.map((plugin) => (
              <li
                key={plugin.pluginSlug}
                className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-border bg-background/40 px-3 py-2"
              >
                <span className="flex min-w-0 items-center gap-2">
                  <span className="truncate font-mono text-xs text-foreground">{plugin.pluginSlug}</span>
                  {!plugin.installed && <Badge variant="outline">{t('audit.coverage.uninstalled')}</Badge>}
                </span>
                <span className="flex items-center gap-3 text-xs text-muted-foreground">
                  <span>{t('audit.coverage.lastSeen', { when: formatDate(plugin.lastSeen) })}</span>
                  <span>{t('audit.coverage.eventCount', { count: plugin.count })}</span>
                </span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}

function Stat({ label, value, tone }: { label: string; value: number; tone?: 'warning' }) {
  return (
    <div className="rounded-md border border-border bg-background/40 px-3 py-2">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className={`text-lg font-semibold ${tone === 'warning' ? 'text-warning' : 'text-foreground'}`}>{value}</p>
    </div>
  );
}
