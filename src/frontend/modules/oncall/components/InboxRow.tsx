// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { Link } from 'react-router';
import { useTranslation } from 'react-i18next';
import { formatDateCustom } from '@resources/utils/format';
import { IconChevronDown, IconChevronUp } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { SeverityBadge } from '@resources/components/ui/SeverityBadge';
import type { InboxItem } from '../hooks/useInbox';
import { NarrativeSnippet } from '@resources/components/incidents/NarrativeSnippet';
import { RunbookBanner } from '@resources/components/runbooks/RunbookBanner';

export function containerLink(item: InboxItem): string | null {
  if (item.sourceType !== 'container' || !item.environmentId || !item.subjectId) return null;
  return `/oncall/containers/${encodeURIComponent(item.environmentId)}/${encodeURIComponent(item.subjectId)}`;
}

export function InboxRow({ item }: { item: InboxItem }) {
  const { t } = useTranslation('oncall');
  const [isOpen, setOpen] = useState(false);
  const link = containerLink(item);
  const whenValue = formatDateCustom(item.createdAt, { dateStyle: 'short', timeStyle: 'short' });
  const where = [item.environmentName, item.subjectName].filter(Boolean).join(' · ');

  return (
    <li className="rounded-lg border border-border bg-card p-4">
      <div className="flex items-center gap-2">
        <SeverityBadge level={item.severity} />
        {item.kind === 'alert' ? (
          <Badge variant={item.state === 'firing' ? 'destructive' : 'secondary'}>
            {item.state === 'firing' ? t('alerts.firing') : t('alerts.resolved')}
          </Badge>
        ) : (
          <Badge variant="secondary">{t('alerts.notification')}</Badge>
        )}
        <time dateTime={item.createdAt} className="ml-auto text-xs text-muted-foreground">
          {whenValue}
        </time>
      </div>
      <p className="mt-2 font-medium">{item.title}</p>
      {where && <p className="text-sm text-muted-foreground">{where}</p>}
      {item.message && (
        <>
          {isOpen && <p className="mt-2 text-sm break-words whitespace-pre-wrap">{item.message}</p>}
          <Button
            variant="ghost"
            size="sm"
            className="mt-1 min-h-11 px-0"
            aria-expanded={isOpen}
            onClick={() => setOpen((value) => !value)}
          >
            {isOpen ? (
              <IconChevronUp className="size-4" aria-hidden="true" />
            ) : (
              <IconChevronDown className="size-4" aria-hidden="true" />
            )}
            {isOpen ? t('alerts.collapse') : t('alerts.expand')}
          </Button>
        </>
      )}
      {item.kind === 'alert' && item.sourceType === 'container' && (
        <NarrativeSnippet alertEventId={item.id.replace(/^alert-/, '')} />
      )}
      {item.kind === 'alert' && (
        <RunbookBanner
          target={{
            environmentId: item.environmentId,
            sourceType: item.sourceType,
            conditionKey: item.conditionKey,
            alertId: item.alertId,
            alertEventId: item.id.replace(/^alert-/, ''),
            container: item.sourceType === 'container' ? item.subjectName : undefined,
          }}
        />
      )}
      {link && (
        <Button asChild variant="outline" className="mt-2 min-h-11 w-full">
          <Link to={link}>{t('alerts.openContainer')}</Link>
        </Button>
      )}
    </li>
  );
}
