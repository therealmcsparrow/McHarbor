// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { useQueries } from '@tanstack/react-query';
import { Command } from 'cmdk';
import { IconDeviceFloppy, IconLoader2 } from '@tabler/icons-react';
import { api } from '@core/api/client';
import type { VolumeInfo } from '@core/types/docker';
import { Dialog, DialogBody, DialogContent, DialogTitle } from '@resources/components/ui/Dialog';
import { useEnvironmentStore } from '@resources/stores/environment';
import { OverviewKeyHints } from './OverviewKeyHints';
import { timeAgo } from '@resources/utils/format';

type VolumeWithEnv = VolumeInfo & { envId: string; envName: string };

type VolumeOverviewProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function VolumeOverview({ open: isOpen, onOpenChange }: VolumeOverviewProps) {
  const { t } = useTranslation('volumes');
  const navigate = useNavigate();
  const environments = useEnvironmentStore((s) => s.environments);

  const dockerEnvs = useMemo(() => environments, [environments]);

  const queries = useQueries({
    queries: dockerEnvs.map((env) => ({
      queryKey: ['volumes-overview', env.id],
      queryFn: () =>
        api.get<VolumeInfo[]>('/volumes', { env: env.id }).then((r) =>
          (r.data ?? []).map((vol) => ({
            ...vol,
            envId: env.id,
            envName: env.name,
          })),
        ),
      refetchInterval: isOpen ? 30_000 : false,
      staleTime: 10_000,
      enabled: isOpen,
    })),
  });

  const isLoading = queries.some((q) => q.isLoading);

  const envGroups = useMemo(() => {
    const groups: {
      envId: string;
      envName: string;
      volumes: VolumeWithEnv[];
    }[] = [];
    for (let i = 0; i < dockerEnvs.length; i++) {
      const env = dockerEnvs[i];
      const volumes = queries[i]?.data ?? [];
      if (env) {
        groups.push({ envId: env.id, envName: env.name, volumes });
      }
    }
    return groups;
  }, [dockerEnvs, queries]);

  const totalCount = envGroups.reduce((sum, g) => sum + g.volumes.length, 0);
  const inUseCount = envGroups.reduce((sum, g) => sum + g.volumes.filter((v) => v.RefCount > 0).length, 0);

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent
        className="top-[12vh] max-h-[76vh] w-[calc(100%-2rem)] max-w-2xl translate-y-0 p-0"
        aria-describedby={undefined}
      >
        <DialogTitle className="sr-only">{t('nav.volumes', { ns: 'common' })}</DialogTitle>
        <DialogBody className="overflow-hidden p-0">
          <Command className="flex flex-col" shouldFilter>
            <div className="flex items-center border-b border-hairline pr-12">
              <IconDeviceFloppy className="ml-4 h-4 w-4 shrink-0 text-muted-foreground" />
              <Command.Input
                placeholder={t('searchPlaceholder')}
                className="w-full bg-transparent px-3 py-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none"
              />
              {isLoading && <IconLoader2 className="mr-4 h-4 w-4 shrink-0 animate-spin text-muted-foreground" />}
            </div>

            <div className="flex items-center gap-3 border-b border-hairline px-4 py-2 text-xs text-muted-foreground">
              <span>{t('volumeOverview.total', { ns: 'common', count: totalCount })}</span>
              <span className="text-success">{t('volumeOverview.inUse', { ns: 'common', count: inUseCount })}</span>
              <span className="ml-auto">
                {t('containerOverview.environments', { ns: 'common', count: dockerEnvs.length })}
              </span>
            </div>

            <Command.List className="max-h-[50vh] overflow-y-auto p-2">
              <Command.Empty className="px-4 py-8 text-center text-sm text-muted-foreground">
                {t('emptyMessage')}
              </Command.Empty>

              {envGroups.map((group) => (
                <Command.Group
                  key={group.envId}
                  heading={`${group.envName} (${group.volumes.length})`}
                  className="px-2 py-1.5 text-xs font-medium tracking-wider text-muted-foreground uppercase"
                >
                  {group.volumes.map((vol) => {
                    const inUse = vol.RefCount > 0;
                    return (
                      <Command.Item
                        key={`${group.envId}-${vol.Name}`}
                        value={`${vol.Name} ${vol.Driver} ${group.envName}`}
                        onSelect={() => {
                          void navigate(`/volumes?env=${group.envId}`);
                          onOpenChange(false);
                        }}
                        className="flex cursor-pointer items-center gap-3 rounded-md px-3 py-2 text-sm text-foreground hover:bg-accent aria-selected:bg-accent"
                      >
                        <span className={`h-2 w-2 shrink-0 rounded-full ${inUse ? 'bg-success' : 'bg-muted'}`} />
                        <span className="min-w-0 truncate font-medium">{vol.Name}</span>
                        <span className="shrink-0 rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">
                          {vol.Driver}
                        </span>
                        <span className="ml-auto shrink-0 text-xs text-muted-foreground">{timeAgo(vol.CreatedAt)}</span>
                      </Command.Item>
                    );
                  })}
                </Command.Group>
              ))}
            </Command.List>

            <OverviewKeyHints />
          </Command>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
