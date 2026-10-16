// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useId } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IconDownload,
  IconPlayerPlay,
  IconPlayerStop,
  IconRegex,
  IconSearch,
  IconSortAscending,
  IconSortDescending,
} from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Select } from '@resources/components/ui/Select';

export type LogFilterBarProps = {
  filter: string;
  onFilterChange: (value: string) => void;
  regexError: boolean;
  regex: boolean;
  onRegexChange: (value: boolean) => void;
  level: string;
  onLevelChange: (value: string) => void;
  timeRange: string;
  onTimeRangeChange: (value: string) => void;
  /** Disables the range when no line carries a wall-clock time to compare against. */
  timeRangeDisabled?: boolean;
  newestFirst: boolean;
  onNewestFirstChange: (value: boolean) => void;
  /** Live is offered only when the caller can stream or poll; omit both props for a static log. */
  live?: boolean;
  onLiveChange?: (value: boolean) => void;
  /** Greys Live out while the source is not producing output and Live is off. */
  isRunning?: boolean;
  /** Export is offered only when the caller provides a handler. */
  onExport?: () => void;
  exportDisabled?: boolean;
  /** Announced to assistive tech whenever the filters change what is shown. */
  visibleCount?: number;
};

export function LogFilterBar(props: LogFilterBarProps) {
  const { t } = useTranslation('common');
  const errorId = useId();
  const live = props.live ?? false;
  const onLiveChange = props.onLiveChange;
  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-col gap-2 xl:flex-row xl:items-center">
        <div className="relative min-w-48 flex-1">
          <IconSearch className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder={t('logs.filterPlaceholder')}
            value={props.filter}
            onChange={(evt) => props.onFilterChange(evt.target.value)}
            maxLength={256}
            aria-label={t('logs.filterLabel')}
            aria-invalid={props.regexError}
            aria-describedby={props.regexError ? errorId : undefined}
            className="min-h-11 pl-9 sm:min-h-0"
          />
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button
            variant={props.regex ? 'default' : 'outline'}
            size="sm"
            onClick={() => props.onRegexChange(!props.regex)}
            aria-pressed={props.regex}
            className="min-h-11 sm:min-h-0"
          >
            <IconRegex className="size-4" /> {t('logs.regex')}
          </Button>
          <Select
            value={props.level}
            onChange={props.onLevelChange}
            searchable={false}
            ariaLabel={t('logs.level')}
            className="min-h-11 w-32 sm:min-h-0"
            options={[
              { value: 'all', label: t('logs.levelAll') },
              { value: 'debug', label: t('logs.levelDebug') },
              { value: 'info', label: t('logs.levelInfo') },
              { value: 'warn', label: t('logs.levelWarn') },
              { value: 'error', label: t('logs.levelError') },
            ]}
          />
          <Select
            value={props.timeRange}
            onChange={props.onTimeRangeChange}
            searchable={false}
            disabled={props.timeRangeDisabled}
            ariaLabel={t('logs.timeRange')}
            className="min-h-11 w-32 sm:min-h-0"
            options={[
              { value: 'all', label: t('logs.allTime') },
              { value: '5m', label: t('logs.last5Minutes') },
              { value: '15m', label: t('logs.last15Minutes') },
              { value: '1h', label: t('logs.lastHour') },
            ]}
          />
          <Button
            variant="outline"
            size="sm"
            onClick={() => props.onNewestFirstChange(!props.newestFirst)}
            className="min-h-11 sm:min-h-0"
          >
            {props.newestFirst ? <IconSortDescending className="size-4" /> : <IconSortAscending className="size-4" />}
            {props.newestFirst ? t('logs.newestFirst') : t('logs.oldestFirst')}
          </Button>
          {onLiveChange ? (
            <Button
              variant={live ? 'default' : 'outline'}
              size="sm"
              onClick={() => onLiveChange(!live)}
              disabled={props.isRunning === false && !live}
              aria-pressed={live}
              className="min-h-11 sm:min-h-0"
            >
              {live ? <IconPlayerStop className="size-4" /> : <IconPlayerPlay className="size-4" />} {t('logs.live')}
            </Button>
          ) : null}
          {props.onExport ? (
            <Button
              variant="outline"
              size="sm"
              onClick={props.onExport}
              disabled={props.exportDisabled}
              className="min-h-11 sm:min-h-0"
            >
              <IconDownload className="size-4" /> {t('logs.export')}
            </Button>
          ) : null}
        </div>
      </div>
      {props.regexError ? (
        <p id={errorId} role="alert" className="text-xs text-destructive">
          {t('logs.invalidRegex')}
        </p>
      ) : null}
      {props.visibleCount !== undefined ? (
        <output className="sr-only" aria-live="polite">
          {t('logs.visibleLineCount', { count: props.visibleCount })}
        </output>
      ) : null}
    </div>
  );
}
