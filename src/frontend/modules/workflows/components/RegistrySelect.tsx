// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { IconChevronDown } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { cn } from '@resources/utils/cn';
import { api, type PaginatedData } from '@core/api/client';

type RegistryItem = {
  id: string;
  name: string;
  url: string;
  username: string;
  isDefault: boolean;
};

interface RegistrySelectProps {
  value: string;
  onChange: (value: string) => void;
}

export function RegistrySelect({ value, onChange }: RegistrySelectProps) {
  const { t } = useTranslation('common');
  const [isOpen, setOpen] = useState(false);

  const { data: registries } = useQuery({
    queryKey: ['registries-select'],
    queryFn: () =>
      api.get<PaginatedData<RegistryItem>>('/registries', { per_page: '100' }).then((r) => r.data?.items ?? []),
    staleTime: 30_000,
  });

  const selected = registries?.find((registry) => registry.id === value);
  const placeholder = t('workflows.selectRegistry');
  const defaultLabel = t('workflows.defaultTransport');
  const useDefaultLabel = t('workflows.useDefaultRegistry');
  const emptyLabel = t('workflows.noRegistriesFound');
  const selectedLabel = value ? (selected?.name ?? placeholder) : useDefaultLabel;

  return (
    <div className="relative">
      <Button
        type="button"
        variant="ghost"
        aria-label={placeholder}
        onClick={() => setOpen((prev) => !prev)}
        className={cn(
          'flex w-full items-center justify-between rounded-md border border-input bg-card px-2 text-xs ring-offset-background focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none',
          !value && 'text-muted-foreground',
        )}
      >
        <span className="truncate">{selectedLabel}</span>
        <IconChevronDown className="size-3 shrink-0 text-muted-foreground" />
      </Button>
      {isOpen && (
        <div className="absolute top-full right-0 left-0 z-50 mt-1 max-h-48 overflow-y-auto rounded-md border border-border bg-popover py-1 shadow-xl">
          <Button
            type="button"
            variant="ghost"
            onClick={() => {
              onChange('');
              setOpen(false);
            }}
            className={cn(
              'flex w-full items-center px-3 py-1.5 text-left text-xs text-muted-foreground transition-colors hover:bg-muted/50',
              !value && 'bg-muted/50 text-foreground',
            )}
          >
            {useDefaultLabel}
          </Button>
          {!registries || registries.length === 0 ? (
            <p className="px-3 py-2 text-xs text-muted-foreground">{emptyLabel}</p>
          ) : (
            registries.map((registry) => (
              <Button
                key={registry.id}
                type="button"
                variant="ghost"
                onClick={() => {
                  onChange(registry.id);
                  setOpen(false);
                }}
                className={cn(
                  'flex h-auto w-full flex-col gap-0.5 px-3 py-1.5 text-left transition-colors hover:bg-muted/50',
                  registry.id === value && 'bg-muted/50',
                )}
              >
                <span className="flex items-center gap-1.5 text-xs font-semibold text-foreground">
                  <span className="truncate">{registry.name}</span>
                  {registry.isDefault && (
                    <span className="bg-primary/10 px-1.5 py-0.5 text-[9px] font-medium tracking-wide text-primary uppercase">
                      {defaultLabel}
                    </span>
                  )}
                </span>
                <span className="truncate text-[10px] text-muted-foreground">
                  {registry.url}
                  {registry.username ? ` - ${registry.username}` : ''}
                </span>
              </Button>
            ))
          )}
        </div>
      )}
    </div>
  );
}
