// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { IconChevronDown } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { cn } from '@resources/utils/cn';
import { api } from '@core/api/client';

type CommunicationChannel = {
  id: string;
  name: string;
  channelType: string;
  isDefault: boolean;
  enabled: boolean;
};

interface CommunicationChannelSelectProps {
  value: string;
  onChange: (value: string) => void;
  channelType?: string;
  incidentOnly?: boolean;
}

export function CommunicationChannelSelect({
  value,
  onChange,
  channelType,
  incidentOnly = false,
}: CommunicationChannelSelectProps) {
  const { t } = useTranslation('common');
  const [isOpen, setIsOpen] = useState(false);

  const { data: channels } = useQuery({
    queryKey: ['communication-channels-select', channelType ?? 'all', incidentOnly],
    queryFn: () =>
      incidentOnly
        ? api.get<CommunicationChannel[]>('/itsm/channels').then((r) => r.data ?? [])
        : api
            .get<CommunicationChannel[]>('/communication-channels')
            .then((r) =>
              (r.data ?? []).filter(
                (channel) => channel.enabled && (!channelType || channel.channelType === channelType),
              ),
            ),
    staleTime: 30_000,
  });

  const selected = channels?.find((channel) => channel.id === value);
  const placeholder = t('workflows.selectCommunicationChannel');
  const defaultLabel = t('workflows.defaultTransport');
  const useDefaultLabel = t('workflows.useDefaultCommunicationChannel');
  const emptyLabel = t('workflows.noCommunicationChannelsFound');
  const selectedLabel = value ? (selected?.name ?? placeholder) : incidentOnly ? placeholder : useDefaultLabel;

  return (
    <div className="relative">
      <Button
        type="button"
        variant="ghost"
        aria-label={placeholder}
        onClick={() => setIsOpen((prev) => !prev)}
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
          {!incidentOnly && (
            <Button
              type="button"
              variant="ghost"
              onClick={() => {
                onChange('');
                setIsOpen(false);
              }}
              className={cn(
                'flex w-full items-center px-3 py-1.5 text-left text-xs text-muted-foreground transition-colors hover:bg-muted/50',
                !value && 'bg-muted/50 text-foreground',
              )}
            >
              {useDefaultLabel}
            </Button>
          )}
          {!channels || channels.length === 0 ? (
            <p className="px-3 py-2 text-xs text-muted-foreground">{emptyLabel}</p>
          ) : (
            channels.map((channel) => (
              <Button
                key={channel.id}
                type="button"
                variant="ghost"
                onClick={() => {
                  onChange(channel.id);
                  setIsOpen(false);
                }}
                className={cn(
                  'flex h-auto w-full flex-col gap-0.5 px-3 py-1.5 text-left transition-colors hover:bg-muted/50',
                  channel.id === value && 'bg-muted/50',
                )}
              >
                <span className="flex items-center gap-1.5 text-xs font-semibold text-foreground">
                  <span className="truncate">{channel.name}</span>
                  {channel.isDefault && (
                    <span className="bg-primary/10 px-1.5 py-0.5 text-[9px] font-medium tracking-wide text-primary uppercase">
                      {defaultLabel}
                    </span>
                  )}
                </span>
                <span className="text-[10px] text-muted-foreground capitalize">{channel.channelType}</span>
              </Button>
            ))
          )}
        </div>
      )}
    </div>
  );
}
