// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { Button } from '@resources/components/ui/Button';
import { cn } from '@resources/utils/cn';
import { api } from '@core/api/client';

type ContainerInfo = {
  Id: string;
  Names: string[];
  State: string;
  Status: string;
  Image: string;
};

interface ContainerSelectProps {
  value: string;
  onChange: (v: string) => void;
  envId?: string;
}

export function ContainerSelect({ value, onChange, envId }: ContainerSelectProps) {
  const { t } = useTranslation('common');
  const [isOpen, setIsOpen] = useState(false);

  const { data: containers } = useQuery({
    queryKey: ['containers-select', envId ?? ''],
    queryFn: () =>
      api
        .get<ContainerInfo[]>('/containers', {
          all: 'true',
          ...(envId ? { env: envId } : {}),
        })
        .then((r) => r.data ?? []),
    staleTime: 30_000,
    enabled: !!envId,
  });

  const selected = containers?.find((c) => c.Id === value || c.Names?.some((n) => n.replace(/^\//, '') === value));
  const displayName = selected
    ? (selected.Names?.[0]?.replace(/^\//, '') ?? selected.Id.slice(0, 12))
    : value || t('workflows.selectContainer');

  return (
    <div className="relative">
      <Button
        type="button"
        variant="ghost"
        aria-label={t('workflows.selectContainer')}
        onClick={() => setIsOpen(!isOpen)}
        className={cn(
          'flex w-full items-center justify-between rounded-md border border-input bg-card px-2 text-xs ring-offset-background focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none',
          !value && 'text-muted-foreground',
        )}
      >
        <span className="truncate">{displayName}</span>
        <svg
          className="size-3 shrink-0 text-muted-foreground"
          viewBox="0 0 12 12"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
        >
          <path d="M3 4.5L6 7.5L9 4.5" />
        </svg>
      </Button>
      {isOpen && (
        <div className="absolute top-full right-0 left-0 z-50 mt-1 max-h-48 overflow-y-auto rounded-md border border-border bg-popover py-1 shadow-xl">
          {!containers || containers.length === 0 ? (
            <p className="px-3 py-2 text-xs text-muted-foreground">{t('workflows.noContainersFound')}</p>
          ) : (
            containers.map((c) => {
              const containerName = c.Names?.[0]?.replace(/^\//, '') ?? '';
              const shortId = c.Id.slice(0, 12);
              const isSelected = c.Id === value || containerName === value;
              return (
                <Button
                  key={c.Id}
                  type="button"
                  variant="ghost"
                  onClick={() => {
                    onChange(containerName || c.Id);
                    setIsOpen(false);
                  }}
                  className={cn(
                    'flex h-auto w-full flex-col gap-0.5 px-3 py-1.5 text-left transition-colors hover:bg-muted/50',
                    isSelected && 'bg-muted/50',
                  )}
                >
                  <span className="text-xs font-semibold text-foreground">{containerName || shortId}</span>
                  <span className="text-[10px] text-muted-foreground">
                    {t('containerSelect.idLabel', { ns: 'workflows', id: shortId })}
                    <span className={cn('ml-2', c.State === 'running' ? 'text-success' : 'text-danger')}>
                      {c.State}
                    </span>
                  </span>
                </Button>
              );
            })
          )}
        </div>
      )}
    </div>
  );
}
