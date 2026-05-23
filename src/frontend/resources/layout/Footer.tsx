// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IconBook,
  IconBrandDocker,
  IconClock,
  IconCpu,
  IconDeviceDesktop,
  IconPlug,
  IconServer,
  IconStack2,
} from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useHostMetrics } from '@modules/dashboard/hooks/useHostMetrics';

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

function Separator() {
  return <span className="text-muted-foreground/20 select-none">|</span>;
}

function useClock() {
  const [now, setNow] = useState(new Date());
  useEffect(() => {
    const id = setInterval(() => setNow(new Date()), 1000);
    return () => clearInterval(id);
  }, []);
  return now;
}

export function Footer() {
  const { t, i18n } = useTranslation('common');
  const now = useClock();
  const { data: metrics } = useHostMetrics();
  const environments = useEnvironmentStore((s) => s.environments);
  const currentId = useEnvironmentStore((s) => s.currentId);
  const env = environments.find((e) => e.id === currentId);

  const timeStr = new Intl.DateTimeFormat(i18n.language, {
    hour: 'numeric',
    minute: '2-digit',
    second: '2-digit',
  }).format(now);

  const connectionLabel = env
    ? t(`environments:connectionTypes.${env.connectionType}`, { defaultValue: env.connectionType })
    : '';

  return (
    <footer className="shrink-0 border-t border-border bg-card/80 backdrop-blur-sm px-4 py-1.5">
      <div className="flex items-center justify-between text-[11px] text-muted-foreground">
        {/* Left: doc button + copyright */}
        <div className="flex items-center gap-1.5 shrink-0">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                aria-label={t('footer.documentation')}
                className="size-6"
                onClick={() => window.open('https://docs.mcharbor.io', '_blank', 'noopener')}
              >
                <IconBook className="size-3.5" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t('footer.documentation')}</TooltipContent>
          </Tooltip>
          <span className="text-muted-foreground/60">© 2026 McSparrow</span>
        </div>

        {/* Center: environment metrics */}
        {metrics && (
          <div className="flex items-center gap-2 text-muted-foreground/50">
            <span className="inline-flex items-center gap-1">
              <IconDeviceDesktop className="size-3 text-sky-400" />
              {metrics.host.os} {formatArch(metrics.host.architecture)}
            </span>
            <Separator />
            <span className="inline-flex items-center gap-1">
              <IconBrandDocker className="size-3 text-blue-400" />
              Docker {metrics.host.serverVersion}
            </span>
            {connectionLabel && (
              <>
                <Separator />
                <span className="inline-flex items-center gap-1">
                  <IconPlug className="size-3 text-emerald-400" />
                  {connectionLabel}
                </span>
              </>
            )}
            <Separator />
            <span className="inline-flex items-center gap-1">
              <IconCpu className="size-3 text-amber-400" />
              {metrics.host.ncpu} {t('footer.cores')}
            </span>
            <Separator />
            <span className="inline-flex items-center gap-1">
              <IconStack2 className="size-3 text-violet-400" />
              {formatBytes(metrics.host.memTotal)} RAM
            </span>
            <Separator />
            <span className="inline-flex items-center gap-1">
              <IconServer className="size-3 text-rose-400" />
              {formatBytes(metrics.disk.total)}
            </span>
            {metrics.host.uptime > 0 && (
              <>
                <Separator />
                <span className="inline-flex items-center gap-1">
                  <IconClock className="size-3 text-teal-400" />
                  {formatUptime(metrics.host.uptime)}
                </span>
              </>
            )}
            <Separator />
            <span className="inline-flex items-center gap-1">
              <IconClock className="size-3 text-orange-400" />
              {timeStr}
            </span>
          </div>
        )}

        {/* Right: version */}
        <span className="shrink-0 text-muted-foreground/60">McHarbor v1.1.6</span>
      </div>
    </footer>
  );
}
