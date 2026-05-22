// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { useQueries } from '@tanstack/react-query';
import { Command } from 'cmdk';
import {
  IconLayoutDashboard, IconBox, IconPhoto, IconDeviceFloppy, IconNetwork, IconStack2,
  IconTerminal, IconFileText, IconWorld, IconBook, IconGitBranch, IconActivity,
  IconClipboardList, IconSettings, IconRefresh, IconInfoCircle, IconSearch, IconLoader2,
} from '@tabler/icons-react';
import { api } from '@core/api/client';
import type { ContainerInfo, ContainerState, ImageInfo, VolumeInfo, NetworkInfo } from '@core/types/docker';
import { useEnvironmentStore } from '@resources/stores/environment';
import { formatBytes } from '@resources/utils/format';
import { Select } from '@resources/components/ui/Select';
import { createSearchMatcher, type SearchMode } from '@resources/utils/search-filter';

// ── Navigation commands ─────────────────────────────────────────────

const NAV_COMMANDS = [
  { labelKey: 'nav.dashboard', to: '/dashboard', icon: IconLayoutDashboard },
  { labelKey: 'nav.containers', to: '/containers', icon: IconBox },
  { labelKey: 'nav.images', to: '/images', icon: IconPhoto },
  { labelKey: 'nav.volumes', to: '/volumes', icon: IconDeviceFloppy },
  { labelKey: 'nav.networks', to: '/networks', icon: IconNetwork },
  { labelKey: 'nav.stacks', to: '/stacks', icon: IconStack2 },
  { labelKey: 'nav.terminal', to: '/terminal', icon: IconTerminal },
  { labelKey: 'nav.logs', to: '/logs', icon: IconFileText },
  { labelKey: 'nav.environments', to: '/environments', icon: IconWorld },
  { labelKey: 'nav.blueprints', to: '/blueprints', icon: IconBook },
  { labelKey: 'nav.git', to: '/git', icon: IconGitBranch },
  { labelKey: 'nav.reconciler', to: '/reconciler', icon: IconRefresh },
  { labelKey: 'nav.activity', to: '/activity', icon: IconActivity },
  { labelKey: 'nav.audit', to: '/audit', icon: IconClipboardList },
  { labelKey: 'nav.settings', to: '/settings', icon: IconSettings },
  { labelKey: 'about.menuItem', to: '/settings?tab=about', icon: IconInfoCircle },
];

// ── State dot colors ────────────────────────────────────────────────

const CONTAINER_DOT: Record<ContainerState, string> = {
  running: 'bg-emerald-500',
  paused: 'bg-amber-500',
  restarting: 'bg-amber-500',
  created: 'bg-zinc-400',
  exited: 'bg-red-500',
  removing: 'bg-red-400',
  dead: 'bg-red-600',
};

const STACK_DOT: Record<string, string> = {
  running: 'bg-emerald-500',
  partial: 'bg-amber-500',
  stopped: 'bg-red-500',
};

// ── Types ───────────────────────────────────────────────────────────

type StackSvc = { name: string; containerId?: string; status: string; image: string };
type StackInfo = { id: string; name: string; status: string; services: StackSvc[]; description?: string; type: 'managed' | 'discovered' };

// ── Helpers ─────────────────────────────────────────────────────────

function containerName(c: ContainerInfo): string {
  const name = c.Names?.[0] ?? c.Id.slice(0, 12);
  return name.startsWith('/') ? name.slice(1) : name;
}

function imageLabel(img: ImageInfo): string {
  const firstTag = img.RepoTags?.[0];
  if (firstTag && firstTag !== '<none>:<none>') return firstTag;
  return img.Id.replace('sha256:', '').slice(0, 12);
}

// ── Component ───────────────────────────────────────────────────────

type GlobalSearchProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function GlobalSearch({ open, onOpenChange }: GlobalSearchProps) {
  const { t } = useTranslation('common');
  const navigate = useNavigate();
  const environments = useEnvironmentStore((s) => s.environments);
  const [query, setQuery] = useState('');
  const [mode, setMode] = useState<SearchMode>('contains');

  const dockerEnvs = useMemo(
    () => environments.filter((e) => e.orchestratorType === 'docker'),
    [environments],
  );

  // Fetch all resource types from all envs
  const containerQueries = useQueries({
    queries: dockerEnvs.map((env) => ({
      queryKey: ['global-search-containers', env.id],
      queryFn: () =>
        api.get<ContainerInfo[]>('/containers', { all: 'true', env: env.id })
          .then((r) => (r.data ?? []).map((c) => ({ ...c, envId: env.id, envName: env.name }))),
      refetchInterval: open ? 15_000 : false,
      staleTime: 5_000,
      enabled: open,
    })),
  });

  const imageQueries = useQueries({
    queries: dockerEnvs.map((env) => ({
      queryKey: ['global-search-images', env.id],
      queryFn: () =>
        api.get<ImageInfo[]>('/images', { env: env.id })
          .then((r) => (r.data ?? []).map((img) => ({ ...img, envId: env.id, envName: env.name }))),
      refetchInterval: open ? 30_000 : false,
      staleTime: 10_000,
      enabled: open,
    })),
  });

  const volumeQueries = useQueries({
    queries: dockerEnvs.map((env) => ({
      queryKey: ['global-search-volumes', env.id],
      queryFn: () =>
        api.get<VolumeInfo[]>('/volumes', { env: env.id })
          .then((r) => (r.data ?? []).map((v) => ({ ...v, envId: env.id, envName: env.name }))),
      refetchInterval: open ? 30_000 : false,
      staleTime: 10_000,
      enabled: open,
    })),
  });

  const networkQueries = useQueries({
    queries: dockerEnvs.map((env) => ({
      queryKey: ['global-search-networks', env.id],
      queryFn: () =>
        api.get<NetworkInfo[]>('/networks', { env: env.id })
          .then((r) => (r.data ?? []).map((n) => ({ ...n, envId: env.id, envName: env.name }))),
      refetchInterval: open ? 30_000 : false,
      staleTime: 10_000,
      enabled: open,
    })),
  });

  const stackQueries = useQueries({
    queries: dockerEnvs.map((env) => ({
      queryKey: ['global-search-stacks', env.id],
      queryFn: () =>
        api.get<StackInfo[]>('/stacks', { env: env.id })
          .then((r) => (r.data ?? []).map((s) => ({ ...s, envId: env.id, envName: env.name }))),
      refetchInterval: open ? 15_000 : false,
      staleTime: 5_000,
      enabled: open,
    })),
  });

  const isLoading =
    containerQueries.some((q) => q.isLoading) ||
    imageQueries.some((q) => q.isLoading) ||
    volumeQueries.some((q) => q.isLoading) ||
    networkQueries.some((q) => q.isLoading) ||
    stackQueries.some((q) => q.isLoading);

  const containers = useMemo(
    () => containerQueries.flatMap((q) => q.data ?? []),
    [containerQueries],
  );
  const images = useMemo(
    () => imageQueries.flatMap((q) => q.data ?? []),
    [imageQueries],
  );
  const volumes = useMemo(
    () => volumeQueries.flatMap((q) => q.data ?? []),
    [volumeQueries],
  );
  const networks = useMemo(
    () => networkQueries.flatMap((q) => q.data ?? []),
    [networkQueries],
  );
  const stacks = useMemo(
    () => stackQueries.flatMap((q) => q.data ?? []),
    [stackQueries],
  );
  const matcher = useMemo(() => createSearchMatcher(query, mode), [mode, query]);
  const filteredNavCommands = useMemo(
    () => NAV_COMMANDS.filter((cmd) => matcher.matches(t(cmd.labelKey))),
    [matcher, t],
  );
  const filteredContainers = useMemo(
    () => containers.filter((c) => matcher.matches(`container ${containerName(c)} ${c.Image} ${c.State} ${c.envName}`)),
    [containers, matcher],
  );
  const filteredImages = useMemo(
    () => images.filter((img) => matcher.matches(`image ${imageLabel(img)} ${img.envName}`)),
    [images, matcher],
  );
  const filteredVolumes = useMemo(
    () => volumes.filter((vol) => matcher.matches(`volume ${vol.Name} ${vol.Driver} ${vol.envName}`)),
    [matcher, volumes],
  );
  const filteredNetworks = useMemo(
    () => networks.filter((net) => matcher.matches(`network ${net.Name} ${net.Driver} ${net.envName}`)),
    [matcher, networks],
  );
  const filteredStacks = useMemo(
    () => stacks.filter((stack) => matcher.matches(`stack ${stack.name} ${stack.status} ${stack.description ?? ''} ${stack.envName}`)),
    [matcher, stacks],
  );

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && open) {
        e.preventDefault();
        onOpenChange(false);
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [open, onOpenChange]);

  useEffect(() => {
    if (!open) {
      setQuery('');
      setMode('contains');
    }
  }, [open]);

  const select = (path: string) => {
    navigate(path);
    onOpenChange(false);
  };

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center pt-[10vh]"
      onClick={() => onOpenChange(false)}
    >
      <div className="fixed inset-0 bg-black/50" />
      <div
        className="relative z-10 w-full max-w-2xl rounded-lg border border-border bg-popover shadow-lg"
        onClick={(e) => e.stopPropagation()}
      >
        <Command className="flex flex-col" shouldFilter={false}>
          <div className="flex items-center border-b border-border">
            <IconSearch className="ml-4 h-4 w-4 shrink-0 text-muted-foreground" />
            <Command.Input
              value={query}
              onValueChange={setQuery}
              placeholder={t('globalSearch.placeholder')}
              className="w-full bg-transparent px-3 py-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none"
              autoFocus
            />
            {isLoading && (
              <IconLoader2 className="mr-4 h-4 w-4 shrink-0 animate-spin text-muted-foreground" />
            )}
          </div>
          <div className="border-b border-border px-3 py-2">
            <Select
              value={mode}
              onChange={(value) => setMode(value as SearchMode)}
              options={[
                { value: 'contains', label: t('filters.modeContains') },
                { value: 'exact', label: t('filters.modeExact') },
                { value: 'regex', label: t('filters.modeRegex') },
              ]}
              searchable={false}
              variant="outline"
              className="w-40"
            />
            {matcher.error && (
              <p className="mt-2 text-xs text-destructive">{t('filters.invalidRegex')}</p>
            )}
          </div>

          <Command.List className="max-h-[60vh] overflow-y-auto p-2">
            {filteredNavCommands.length === 0 &&
              filteredContainers.length === 0 &&
              filteredImages.length === 0 &&
              filteredVolumes.length === 0 &&
              filteredNetworks.length === 0 &&
              filteredStacks.length === 0 && (
              <Command.Empty className="px-4 py-8 text-center text-sm text-muted-foreground">
                {t('globalSearch.noResults')}
              </Command.Empty>
            )}

            {filteredNavCommands.length > 0 && (
              <Command.Group
                heading={t('nav.navigation')}
                className="px-2 py-1 text-xs font-medium text-muted-foreground"
              >
                {filteredNavCommands.map((cmd) => {
                  const label = t(cmd.labelKey);
                  return (
                    <Command.Item
                      key={cmd.to}
                      value={`nav ${label}`}
                      onSelect={() => select(cmd.to)}
                      className="flex cursor-pointer items-center gap-3 rounded-md px-3 py-2 text-sm text-foreground hover:bg-accent aria-selected:bg-accent"
                    >
                      <cmd.icon className="h-4 w-4 shrink-0 text-muted-foreground" />
                      {label}
                    </Command.Item>
                  );
                })}
              </Command.Group>
            )}

            {filteredContainers.length > 0 && (
              <Command.Group
                heading={t('globalSearch.containers', { count: filteredContainers.length })}
                className="px-2 py-1 text-xs font-medium text-muted-foreground"
              >
                {filteredContainers.map((c) => {
                  const name = containerName(c);
                  const dotClass = CONTAINER_DOT[c.State] ?? 'bg-zinc-400';
                  return (
                    <Command.Item
                      key={`c-${c.envId}-${c.Id}`}
                      value={`container ${name} ${c.Image} ${c.State} ${c.envName}`}
                      onSelect={() => select(`/containers/${c.Id}?env=${c.envId}`)}
                      className="flex cursor-pointer items-center gap-3 rounded-md px-3 py-2 text-sm text-foreground hover:bg-accent aria-selected:bg-accent"
                    >
                      <span className={`h-2 w-2 shrink-0 rounded-full ${dotClass}`} />
                      <span className="min-w-0 truncate font-medium">{name}</span>
                      <span className="min-w-0 truncate text-xs text-muted-foreground">{c.Image}</span>
                      <EnvTag name={c.envName} />
                    </Command.Item>
                  );
                })}
              </Command.Group>
            )}

            {filteredImages.length > 0 && (
              <Command.Group
                heading={t('globalSearch.images', { count: filteredImages.length })}
                className="px-2 py-1 text-xs font-medium text-muted-foreground"
              >
                {filteredImages.map((img) => {
                  const label = imageLabel(img);
                  const inUse = img.Containers > 0;
                  return (
                    <Command.Item
                      key={`i-${img.envId}-${img.Id}`}
                      value={`image ${label} ${img.envName}`}
                      onSelect={() => select(`/images/${encodeURIComponent(img.Id)}?env=${img.envId}`)}
                      className="flex cursor-pointer items-center gap-3 rounded-md px-3 py-2 text-sm text-foreground hover:bg-accent aria-selected:bg-accent"
                    >
                      <span className={`h-2 w-2 shrink-0 rounded-full ${inUse ? 'bg-emerald-500' : 'bg-zinc-400'}`} />
                      <span className="min-w-0 truncate font-medium">{label}</span>
                      <span className="text-xs tabular-nums text-muted-foreground">{formatBytes(img.Size)}</span>
                      <EnvTag name={img.envName} />
                    </Command.Item>
                  );
                })}
              </Command.Group>
            )}

            {filteredVolumes.length > 0 && (
              <Command.Group
                heading={t('globalSearch.volumes', { count: filteredVolumes.length })}
                className="px-2 py-1 text-xs font-medium text-muted-foreground"
              >
                {filteredVolumes.map((vol) => {
                  const inUse = vol.RefCount > 0;
                  return (
                    <Command.Item
                      key={`v-${vol.envId}-${vol.Name}`}
                      value={`volume ${vol.Name} ${vol.Driver} ${vol.envName}`}
                      onSelect={() => select(`/volumes?env=${vol.envId}`)}
                      className="flex cursor-pointer items-center gap-3 rounded-md px-3 py-2 text-sm text-foreground hover:bg-accent aria-selected:bg-accent"
                    >
                      <span className={`h-2 w-2 shrink-0 rounded-full ${inUse ? 'bg-emerald-500' : 'bg-zinc-400'}`} />
                      <span className="min-w-0 truncate font-medium">{vol.Name}</span>
                      <span className="shrink-0 rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">{vol.Driver}</span>
                      <EnvTag name={vol.envName} />
                    </Command.Item>
                  );
                })}
              </Command.Group>
            )}

            {filteredNetworks.length > 0 && (
              <Command.Group
                heading={t('globalSearch.networks', { count: filteredNetworks.length })}
                className="px-2 py-1 text-xs font-medium text-muted-foreground"
              >
                {filteredNetworks.map((net) => (
                  <Command.Item
                    key={`n-${net.envId}-${net.Id}`}
                    value={`network ${net.Name} ${net.Driver} ${net.envName}`}
                    onSelect={() => select(`/networks/${net.Id}?env=${net.envId}`)}
                    className="flex cursor-pointer items-center gap-3 rounded-md px-3 py-2 text-sm text-foreground hover:bg-accent aria-selected:bg-accent"
                  >
                    <span className="h-2 w-2 shrink-0 rounded-full bg-blue-500" />
                    <span className="min-w-0 truncate font-medium">{net.Name}</span>
                    <span className="shrink-0 rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">{net.Driver}</span>
                    <EnvTag name={net.envName} />
                  </Command.Item>
                ))}
              </Command.Group>
            )}

            {filteredStacks.length > 0 && (
              <Command.Group
                heading={t('globalSearch.stacks', { count: filteredStacks.length })}
                className="px-2 py-1 text-xs font-medium text-muted-foreground"
              >
                {filteredStacks.map((stack) => {
                  const dotClass = STACK_DOT[stack.status] ?? 'bg-zinc-400';
                  return (
                    <Command.Item
                      key={`s-${stack.envId}-${stack.name}`}
                      value={`stack ${stack.name} ${stack.status} ${stack.description ?? ''} ${stack.envName}`}
                      onSelect={() => select(`/stacks/${encodeURIComponent(stack.name)}?env=${stack.envId}`)}
                      className="flex cursor-pointer items-center gap-3 rounded-md px-3 py-2 text-sm text-foreground hover:bg-accent aria-selected:bg-accent"
                    >
                      <span className={`h-2 w-2 shrink-0 rounded-full ${dotClass}`} />
                      <span className="min-w-0 truncate font-medium">{stack.name}</span>
                      <span className="shrink-0 text-xs text-muted-foreground">{stack.services.length} svc</span>
                      <EnvTag name={stack.envName} />
                    </Command.Item>
                  );
                })}
              </Command.Group>
            )}
          </Command.List>

          <div className="flex items-center gap-4 border-t border-border px-4 py-2 text-xs text-muted-foreground">
            <span className="flex items-center gap-1.5">
              <kbd className="rounded border border-border bg-muted px-1.5 py-0.5 font-mono text-[10px]">
                ↑↓
              </kbd>
              {t('globalSearch.navigate')}
            </span>
            <span className="flex items-center gap-1.5">
              <kbd className="rounded border border-border bg-muted px-1.5 py-0.5 font-mono text-[10px]">
                ↵
              </kbd>
              {t('globalSearch.open')}
            </span>
            <span className="flex items-center gap-1.5">
              <kbd className="rounded border border-border bg-muted px-1.5 py-0.5 font-mono text-[10px]">
                esc
              </kbd>
              {t('globalSearch.close')}
            </span>
          </div>
        </Command>
      </div>
    </div>
  );
}

function EnvTag({ name }: { name: string }) {
  return (
    <span className="ml-auto shrink-0 rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">
      {name}
    </span>
  );
}

