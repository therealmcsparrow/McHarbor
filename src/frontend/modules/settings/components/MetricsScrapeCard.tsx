// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { IconActivity, IconCopy, IconPlayerPlay } from '@tabler/icons-react';
import { ManualCopyDialog } from '@resources/components/ManualCopyDialog';
import { Button } from '@resources/components/ui/Button';
import { SettingsGroup } from '@resources/components/settings/SettingsGroup';
import { copyToClipboard } from '@resources/utils/clipboard';
import { PERM } from '@core/auth/permissionKeys';
import { useTestScrape } from '../hooks/useMetricsScrape';

/**
 * Settings → Data retention → Prometheus metrics. McHarbor's own metrics are
 * scraped with an API key, not a browser session, so the card shows the URL a
 * scraper needs and lets an operator prove the route answers.
 */
export function MetricsScrapeCard() {
  const { t } = useTranslation('settings');
  const { t: tc } = useTranslation('common');
  const scrape = useTestScrape();
  const [manualCopy, setManualCopy] = useState(false);
  const endpoint = `${window.location.origin}/api/metrics`;

  const copyEndpoint = async () => {
    const result = await copyToClipboard(endpoint);
    if (!result.ok) {
      setManualCopy(true);
      return;
    }
    toast.success(tc('clipboard.copied'));
  };

  return (
    <SettingsGroup
      icon={IconActivity}
      title={t('retention.metricsScrapeTitle')}
      description={t('retention.metricsScrapeDescription')}
      scope={t('general.scopeGlobal')}
    >
      <div className="space-y-1.5">
        <p className="text-xs font-medium text-foreground">{t('retention.metricsScrapeEndpoint')}</p>
        <div className="flex flex-wrap items-center gap-2">
          <code className="min-w-0 flex-1 truncate rounded-md border border-border bg-background/40 px-3 py-2 font-mono text-xs text-foreground">
            {endpoint}
          </code>
          <Button
            type="button"
            variant="outline"
            size="icon"
            aria-label={t('retention.metricsScrapeCopy')}
            onClick={copyEndpoint}
          >
            <IconCopy className="size-4" aria-hidden="true" />
          </Button>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-3">
        <Button
          permission={PERM.METRICS_SCRAPE}
          type="button"
          variant="outline"
          size="sm"
          onClick={() => scrape.mutate()}
          disabled={scrape.isPending}
        >
          <IconPlayerPlay className="size-4" aria-hidden="true" />
          {scrape.isPending ? t('retention.metricsScrapeTesting') : t('retention.metricsScrapeTest')}
        </Button>
        {scrape.isSuccess && (
          <output className="block text-xs text-success">
            {t('retention.metricsScrapeOk', { count: scrape.data.lines })}
          </output>
        )}
        {scrape.isError && (
          <p role="alert" className="text-xs break-words whitespace-pre-wrap text-destructive">
            {scrape.error.message}
          </p>
        )}
      </div>

      <ManualCopyDialog open={manualCopy} onOpenChange={setManualCopy} value={endpoint} />
    </SettingsGroup>
  );
}
