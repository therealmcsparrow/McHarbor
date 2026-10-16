// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import {
  IconBrandGithub,
  IconCheck,
  IconCopy,
  IconMail,
  IconShieldLock,
  IconWorld,
  type TablerIcon,
} from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { ManualCopyDialog } from '@resources/components/ManualCopyDialog';
import { copyToClipboard } from '@resources/utils/clipboard';
import { useFlash } from '@resources/hooks/useFlash';
import { assertSuccess } from '@resources/utils/api-mutation';
import { api } from '@core/api/client';

type AboutData = {
  version: string;
  goVersion: string;
  platform: string;
};

// The three constants AboutTab already carries, plus the address LICENSE
// publishes. Deliberately not configurable: an operator who changes where
// support requests go is changing who maintains their fork, not a setting.
const websiteUrl = 'https://www.mcharbor.eu';
const repositoryUrl = 'https://github.com/therealmcsparrow/mcharbor';
const issuesUrl = `${repositoryUrl}/issues/new`;
const contactEmail = 'info@mcsparrow.eu';
const EXTERNAL_LINK_PROPS = { target: '_blank', rel: 'noreferrer' } as const;

type Channel = {
  key: 'email' | 'issues' | 'website';
  icon: TablerIcon;
  href: string;
  external: boolean;
};

const channels: Channel[] = [
  { key: 'email', icon: IconMail, href: `mailto:${contactEmail}`, external: false },
  { key: 'issues', icon: IconBrandGithub, href: issuesUrl, external: true },
  { key: 'website', icon: IconWorld, href: websiteUrl, external: true },
];

export function ContactTab() {
  const { t } = useTranslation('settings');

  const { data, error, refetch } = useQuery({
    queryKey: ['about'],
    queryFn: () => api.get<AboutData>('/about').then(assertSuccess),
    staleTime: 60_000,
  });

  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-xl font-bold text-foreground">{t('contact.title')}</h2>
        <p className="mt-1 max-w-3xl text-sm text-muted-foreground">{t('contact.intro')}</p>
      </div>

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {channels.map((channel) => (
          <ChannelCard key={channel.key} channel={channel} />
        ))}
      </div>

      <div className="rounded-lg border border-border bg-muted/40 p-5">
        <div className="flex items-start gap-3">
          <IconShieldLock className="mt-0.5 size-5 shrink-0 text-muted-foreground" />
          <div>
            <h3 className="text-sm font-semibold text-foreground">{t('contact.security.title')}</h3>
            <p className="mt-1 max-w-3xl text-sm text-muted-foreground">
              {t('contact.security.description', { address: contactEmail })}
            </p>
          </div>
        </div>
      </div>

      {error ? (
        <ErrorState variant="inline" error={error} onRetry={() => void refetch()} />
      ) : (
        <DiagnosticsCard about={data} />
      )}

      <div className="border-t border-border pt-4 text-sm text-muted-foreground">
        {t('about.copyrightHolder')} {t('about.allRightsReserved')}
      </div>
    </div>
  );
}

function ChannelCard({ channel }: { channel: Channel }) {
  const { t } = useTranslation('settings');
  const Icon = channel.icon;

  return (
    <div className="flex flex-col rounded-lg border border-border bg-card p-4">
      <div className="flex items-center gap-3">
        <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-muted">
          <Icon className="size-5 text-foreground" />
        </span>
        <h3 className="text-sm font-semibold text-foreground">{t(`contact.${channel.key}.title`)}</h3>
      </div>
      <p className="mt-3 grow text-sm text-muted-foreground">{t(`contact.${channel.key}.description`)}</p>
      <div className="mt-4">
        <Button variant="outline" asChild>
          <a href={channel.href} {...(channel.external ? EXTERNAL_LINK_PROPS : {})}>
            <Icon className="size-4" />
            {t(`contact.${channel.key}.action`)}
          </a>
        </Button>
      </div>
    </div>
  );
}

// DiagnosticsCard renders the block an operator is asked to paste into a bug
// report. Everything in it is already visible on the About section next to
// this one — version, Go version, platform — plus the two browser facts a
// maintainer always has to ask for. Nothing here reaches for hostnames,
// tokens, environment names or container data: a support block people are
// afraid to paste is a support block nobody pastes.
function DiagnosticsCard({ about }: { about: AboutData | undefined }) {
  const { t, i18n } = useTranslation('settings');
  const { flashing: copied, flash: flashCopied } = useFlash();
  const [manualOpen, setManualOpen] = useState(false);

  const userAgent = typeof navigator === 'undefined' ? '-' : navigator.userAgent;
  const rows: { label: string; value: string }[] = [
    { label: t('about.version'), value: about ? `v${about.version}` : '-' },
    { label: 'Go', value: about ? about.goVersion.replace('go', '') : '-' },
    { label: t('about.platform'), value: about?.platform ?? '-' },
    { label: t('contact.diagnostics.browser'), value: userAgent },
    { label: t('contact.diagnostics.language'), value: i18n.language },
  ];

  const report = rows.map((row) => `${row.label}: ${row.value}`).join('\n');

  async function copy() {
    const result = await copyToClipboard(report);
    if (!result.ok) {
      setManualOpen(true);
      return;
    }
    flashCopied();
  }

  return (
    <div className="rounded-lg border border-border">
      <div className="flex flex-wrap items-start justify-between gap-4 border-b border-border p-5">
        <div>
          <h3 className="text-sm font-semibold text-foreground">{t('contact.diagnostics.title')}</h3>
          <p className="mt-1 max-w-2xl text-sm text-muted-foreground">{t('contact.diagnostics.description')}</p>
        </div>
        <Button variant="outline" onClick={copy}>
          {copied ? <IconCheck className="size-4" /> : <IconCopy className="size-4" />}
          {copied ? t('contact.diagnostics.copied') : t('contact.diagnostics.copy')}
        </Button>
      </div>

      <div className="grid grid-cols-1 gap-px bg-border sm:grid-cols-2">
        {rows.map((row) => (
          <div key={row.label} className="flex items-baseline justify-between gap-3 bg-card px-4 py-2.5">
            <span className="shrink-0 text-sm text-muted-foreground">{row.label}</span>
            <span className="min-w-0 text-right font-mono text-xs break-all text-foreground">{row.value}</span>
          </div>
        ))}
      </div>

      <ManualCopyDialog open={manualOpen} onOpenChange={setManualOpen} value={report} />
    </div>
  );
}
