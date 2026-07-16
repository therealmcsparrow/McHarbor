// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Link } from 'react-router';
import { useTranslation } from 'react-i18next';
import {
  IconBox,
  IconHistory,
  IconDeviceFloppy,
  IconNetwork,
  IconStack2,
  IconPhoto,
  IconChevronDown,
  IconChevronRight,
  type TablerIcon,
} from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { formatDate } from '@resources/utils/format';
import { severityStyle } from '@resources/utils/severity';
import type { LocaleFormat } from '@resources/utils/format';
import type { LifecycleEvent, LifecycleSubjectType, LifecycleSeverity } from '../hooks/useLifecycle';

const SUBJECT_ICON: Record<LifecycleSubjectType, TablerIcon> = {
  container: IconBox,
  image: IconPhoto,
  volume: IconDeviceFloppy,
  network: IconNetwork,
  stack: IconStack2,
};

const SEVERITY_LABEL: Record<LifecycleSeverity, string> = {
  success: 'lifecycle.severity.success',
  info: 'lifecycle.severity.info',
  warning: 'lifecycle.severity.warning',
  error: 'lifecycle.severity.error',
};

// EnvironmentLite is the structural subset LifecycleRow needs from
// the global environment store. Kept inline so the store's private
// `Environment` type does not need to be exported.
type EnvironmentLite = { id: string; name: string };

type LifecycleRowProps = {
  event: LifecycleEvent;
  expanded: boolean;
  onToggle: (id: string) => void;
  formatPrefs: LocaleFormat;
  environments: EnvironmentLite[];
};

export function LifecycleRow({
  event,
  expanded,
  onToggle,
  formatPrefs,
  environments,
}: LifecycleRowProps) {
  const { t } = useTranslation('settings');
  const meta = parseMetadata(event.metadata);
  const SubjectIcon = SUBJECT_ICON[event.subjectType] ?? IconHistory;
  const style = severityStyle(event.severity);
  const envName =
    environments.find((e) => e.id === event.environmentId)?.name ??
    event.environmentId ??
    t('lifecycle.localEnvironment');
  return (
    <li className="px-4 py-2.5 text-sm">
      <div className="grid grid-cols-[auto_140px_1fr_140px_140px] items-center gap-3">
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          onClick={() => onToggle(event.id)}
          className="text-muted-foreground hover:text-foreground"
          aria-label={expanded ? t('lifecycle.collapse') : t('lifecycle.expand')}
          aria-expanded={expanded}
        >
          {expanded ? (
            <IconChevronDown className="size-4" />
          ) : (
            <IconChevronRight className="size-4" />
          )}
        </Button>
        <div className="flex items-center gap-2 truncate">
          <SubjectIcon className="size-4 shrink-0 text-muted-foreground" />
          <div className="min-w-0">
            <Link
              to={buildSubjectPath(event.environmentId, event)}
              className="block truncate text-foreground hover:text-primary"
              title={event.subjectName ?? event.subjectId}
            >
              {event.subjectName ?? event.subjectId.slice(0, 12)}
            </Link>
            <div className="truncate text-xs text-muted-foreground">
              {event.subjectType} · {envName}
            </div>
          </div>
        </div>
        <div className="min-w-0">
          <div className="truncate font-medium">
            {humanizeAction(event.action, t)}
          </div>
          <div className="truncate text-xs text-muted-foreground">
            {event.state
              ? t(`lifecycle.state.${event.state}`, { defaultValue: event.state })
              : event.eventType}
          </div>
        </div>
        <div>
          <span
            className={`inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium capitalize ${style.pill} ${style.text}`}
            aria-label={t(SEVERITY_LABEL[event.severity] ?? 'lifecycle.severity.info')}
          >
            <span className={`size-1.5 rounded-full ${style.dot}`} aria-hidden="true" />
            {t(SEVERITY_LABEL[event.severity] ?? 'lifecycle.severity.info')}
          </span>
        </div>
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="cursor-help text-right font-mono text-xs text-muted-foreground">
              {formatDate(event.timestamp, formatPrefs)}
            </span>
          </TooltipTrigger>
          <TooltipContent>
            {t('lifecycle.source', { source: event.source })}
          </TooltipContent>
        </Tooltip>
      </div>
      {expanded && (
        <div className="ml-7 mt-2 rounded-md border border-border bg-muted/30 p-3 text-xs">
          <div className="grid gap-1 sm:grid-cols-2">
            <div>
              <div className="text-muted-foreground">{t('lifecycle.detailId')}</div>
              <div className="font-mono">{event.id}</div>
            </div>
            <div>
              <div className="text-muted-foreground">{t('lifecycle.detailSubject')}</div>
              <div className="font-mono">{event.subjectType} / {event.subjectId}</div>
            </div>
            <div>
              <div className="text-muted-foreground">{t('lifecycle.detailEvent')}</div>
              <div className="font-mono">{event.eventType} / {event.action}</div>
            </div>
            <div>
              <div className="text-muted-foreground">{t('lifecycle.detailState')}</div>
              <div className="font-mono">{event.state ?? '—'}</div>
            </div>
          </div>
          {meta && Object.keys(meta).length > 0 && (
            <div className="mt-2">
              <div className="mb-1 text-muted-foreground">{t('lifecycle.detailAttributes')}</div>
              <pre className="max-h-48 overflow-auto rounded border border-border bg-background p-2 font-mono text-[11px] leading-relaxed">
                {Object.entries(meta)
                  .map(([k, v]) => `${k}=${v}`)
                  .join('\n')}
              </pre>
            </div>
          )}
        </div>
      )}
    </li>
  );
}

function parseMetadata(raw: string | undefined): Record<string, string> | null {
  if (!raw) return null;
  try {
    const obj = JSON.parse(raw);
    if (obj && typeof obj === 'object') return obj as Record<string, string>;
  } catch {
    // fall through
  }
  return null;
}

function buildSubjectPath(envId: string | undefined, subject: LifecycleEvent): string {
  switch (subject.subjectType) {
    case 'container':
      return `/${envId ? '' : ''}containers/${subject.subjectId}`;
    case 'image':
      return `/images/${subject.subjectId}`;
    case 'volume':
      return `/volumes/${subject.subjectId}`;
    case 'network':
      return `/networks/${subject.subjectId}`;
    default:
      return '#';
  }
}

function humanizeAction(action: string, t: (key: string) => string): string {
  const translated = t(`lifecycle.action.${action}`);
  return translated === `lifecycle.action.${action}` ? action : translated;
}

type LifecyclePaginationProps = {
  page: number;
  totalPages: number;
  total: number;
  disabled: boolean;
  onPageChange: (page: number) => void;
  totalLabel: string;
  pageOfLabel: string;
  previousLabel: string;
  nextLabel: string;
};

export function LifecyclePagination({
  page,
  totalPages,
  total,
  disabled,
  onPageChange,
  totalLabel,
  pageOfLabel,
  previousLabel,
  nextLabel,
}: LifecyclePaginationProps) {
  if (total <= 0) return null;
  return (
    <div className="flex items-center justify-between text-xs text-muted-foreground">
      <div>{totalLabel.replace('{total}', String(total))}</div>
      <div className="flex items-center gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={page <= 1 || disabled}
          onClick={() => onPageChange(Math.max(1, page - 1))}
        >
          {previousLabel}
        </Button>
        <span>
          {pageOfLabel.replace('{page}', String(page)).replace('{totalPages}', String(totalPages))}
        </span>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={page >= totalPages || disabled}
          onClick={() => onPageChange(Math.min(totalPages, page + 1))}
        >
          {nextLabel}
        </Button>
      </div>
    </div>
  );
}