// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import {
  IconBook,
  IconBrandDocker,
  IconClock,
  IconCpu,
  IconDeviceDesktop,
  IconServer,
  IconStack2,
} from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useHostMetrics } from '@resources/hooks/useHostMetrics';
import { api } from '@core/api/client';
import { cn } from '@resources/utils/cn';
import { assertSuccess } from '@resources/utils/api-mutation';

const COPYRIGHT_NOTICE = '© 2026 McSparrow';
const PRODUCT_NAME = 'McHarbor';
const DOCKER_NAME = 'Docker';

const ARCH_LABELS: Record<string, string> = {
  x86_64: 'x64',
  amd64: 'x64',
  aarch64: 'arm64',
  arm64: 'arm64',
};

function formatArch(arch: string): string {
  return ARCH_LABELS[arch.toLowerCase()] ?? arch;
}

function formatBytes(bytes: number): string {
  if (bytes < 1024 ** 3) return `${(bytes / 1024 ** 2).toFixed(0)} MB`;
  return `${(bytes / 1024 ** 3).toFixed(1)} GB`;
}

function formatUptime(seconds: number): string {
  if (seconds <= 0) return '';
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const mins = Math.floor((seconds % 3600) / 60);
  if (days > 0) return `${days}d ${hours}h`;
  if (hours > 0) return `${hours}h ${mins}m`;
  return `${mins}m`;
}

function Cell({ children, className }: { children: React.ReactNode; className?: string }) {
  return (
    <div className={cn('flex h-full shrink-0 items-center gap-1.5 border-r border-border px-3', className)}>
      {children}
    </div>
  );
}

function useClock() {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const id = setInterval(() => setNow(new Date()), 1000);
    return () => clearInterval(id);
  }, []);
  return now;
}

export function Footer() {
  const { t, i18n } = useTranslation('common');
  const now = useClock();
  const { data: metrics, isError: metricsError } = useHostMetrics();
  const { data: info } = useQuery({
    queryKey: ['about'],
    queryFn: () => api.get<{ version: string }>('/about').then(assertSuccess),
    staleTime: 60_000,
  });
  const environments = useEnvironmentStore((s) => s.environments);
  const currentId = useEnvironmentStore((s) => s.currentId);
  const env = environments.find((e) => e.id === currentId);
  // Metrics arriving at all is the signal that the Docker endpoint is
  // reachable; a failing query is the signal that it is not.
  const connectionHealthy = Boolean(metrics) && !metricsError;

  const timeStr = new Intl.DateTimeFormat(i18n.language, {
    hour: 'numeric',
    minute: '2-digit',
    second: '2-digit',
  }).format(now);

  const connectionLabel = env ? t(`environments:connectionTypes.${env.connectionType}`) : '';

  return (
    <footer className="relative z-10 h-7 shrink-0 border-t border-border bg-card">
      <div className="flex h-full items-stretch text-[11px] text-muted-foreground">
        {/* Left: docs and copyright */}
        <div className="flex h-full min-w-0 flex-1 items-stretch">
          <Cell>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={t('footer.documentation')}
                  className="size-5 rounded-none"
                  onClick={() => window.open('https://www.mcharbor.eu/docs', '_blank', 'noopener')}
                >
                  <IconBook className="size-3.5" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>{t('footer.documentation')}</TooltipContent>
            </Tooltip>
            <span className="text-muted-foreground/60">{COPYRIGHT_NOTICE}</span>
          </Cell>
        </div>

        {/* Center: environment metrics.
            These eight facts used to carry eight different hues — a blue
            monitor, a green plug, an amber CPU, a violet stack, a red disk.
            None of them encoded state: the CPU count is 8 whether things
            are going well or badly, so colouring it spent the reader's
            attention on something that never changes.
            They are all muted ink now, and exactly one element in this
            strip stays coloured: the connection pip, which is the only
            thing here that is actually reporting a condition. */}
        {metrics && (
          <div className="flex h-full shrink-0 items-stretch border-l border-border">
            {connectionLabel && (
              <Cell>
                {/* The one coloured element in the strip. It pulses only
                    while connected — `mc-status-pulse` is gated on the
                    motion preference, so it holds still for anyone who
                    asked for that. */}
                <output
                  aria-label={connectionHealthy ? t('footer.connected') : t('footer.disconnected')}
                  className={cn('size-1.5', connectionHealthy ? 'mc-status-pulse bg-success' : 'bg-danger')}
                />
                {env?.name && <span className="font-semibold text-foreground">{env.name}</span>}
                <span className="text-muted-foreground/70">{connectionLabel}</span>
              </Cell>
            )}
            <Cell className="text-muted-foreground/70">
              <IconDeviceDesktop className="size-3" />
              {metrics.host.os} {formatArch(metrics.host.architecture)}
            </Cell>
            <Cell className="text-muted-foreground/70">
              <IconBrandDocker className="size-3" />
              {DOCKER_NAME} {metrics.host.serverVersion}
            </Cell>
            <Cell className="text-muted-foreground/70">
              <IconCpu className="size-3" />
              {metrics.host.ncpu} {t('footer.cores')}
            </Cell>
            <Cell className="text-muted-foreground/70">
              <IconStack2 className="size-3" />
              {formatBytes(metrics.host.memTotal)} RAM
            </Cell>
            <Cell className="text-muted-foreground/70">
              <IconServer className="size-3" />
              {formatBytes(metrics.disk.total)}
            </Cell>
            {metrics.host.uptime > 0 && (
              <Cell className="text-muted-foreground/70">
                <IconClock className="size-3" />
                {formatUptime(metrics.host.uptime)}
              </Cell>
            )}
            <Cell className="text-muted-foreground/70 tabular-nums">
              <IconClock className="size-3" />
              {timeStr}
            </Cell>
          </div>
        )}

        {/* Right: version */}
        <div className="flex h-full min-w-0 flex-1 items-stretch justify-end">
          <Cell className="border-r-0 border-l border-border">
            <span className="text-muted-foreground/60">
              {info?.version ? t('footer.version', { version: info.version }) : PRODUCT_NAME}
            </span>
          </Cell>
        </div>
      </div>
    </footer>
  );
}
