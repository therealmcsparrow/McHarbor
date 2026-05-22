// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { IconPlus, IconTrash, IconPower, IconPlugOff } from '@tabler/icons-react';
import { api, type PaginatedData } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { useEnvironmentStore } from '@resources/stores/environment';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { ListItem } from '@resources/components/ui/ListItem';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Spinner } from '@resources/components/ui/Spinner';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@resources/components/ui/Dialog';
import type { PluginItem } from '../types';

export function PluginsTab() {
  const { t } = useTranslation('settings');
  const { t: tc } = useTranslation('common');
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);

  const { data: plugins = [], isLoading } = useQuery({
    queryKey: ['plugins', envId],
    queryFn: () =>
      api
        .get<PaginatedData<PluginItem>>('/plugins', { per_page: '100', ...(envId ? { env: envId } : {}) })
        .then((r) => r.data?.items ?? []),
  });

  const installPlugin = useMutation({
    mutationFn: (data: { name: string; version: string; source: string }) =>
      api.post('/plugins', data).then(assertSuccess),
    meta: { success: t('toast.pluginInstalled') },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['plugins'] }),
  });

  const togglePlugin = useMutation({
    mutationFn: (id: string) => api.post(`/plugins/${id}/toggle`).then(assertSuccess),
    meta: { success: t('toast.pluginUpdated') },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['plugins'] }),
  });

  const uninstallPlugin = useMutation({
    mutationFn: (id: string) => api.del(`/plugins/${id}`).then(assertSuccess),
    meta: { success: t('toast.pluginUninstalled') },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['plugins'] }),
  });

  const [installOpen, setInstallOpen] = useState(false);
  const [plName, setPlName] = useState('');
  const [plVersion, setPlVersion] = useState('');
  const [plSource, setPlSource] = useState('');

  const handleInstall = () => {
    if (!plName.trim() || !plSource.trim()) return;
    installPlugin.mutate(
      { name: plName.trim(), version: plVersion.trim(), source: plSource.trim() },
      {
        onSuccess: () => {
          setInstallOpen(false);
          setPlName('');
          setPlVersion('');
          setPlSource('');
        },
      }
    );
  };

  if (isLoading) {
    return (
      <div className="flex h-32 items-center justify-center">
        <Spinner />
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <p className="text-sm text-muted-foreground">
          {t('plugins.description')}
        </p>
        <Button size="sm" onClick={() => setInstallOpen(true)}>
          <IconPlus className="h-4 w-4" /> {t('plugins.installPlugin')}
        </Button>
      </div>

      {plugins.length === 0 ? (
        <div className="py-8 text-center text-muted-foreground">{t('plugins.noPlugins')}</div>
      ) : (
        <div className="space-y-2">
          {plugins.map((pl) => (
            <ListItem
              key={pl.id}
              title={pl.name}
              subtitle={pl.description || undefined}
              badge={
                <>
                  {pl.version && (
                    <span className="text-xs text-muted-foreground">v{pl.version}</span>
                  )}
                  <Badge variant={pl.enabled ? 'success' : 'secondary'}>
                    {pl.enabled ? t('plugins.enabled') : t('plugins.disabled')}
                  </Badge>
                </>
              }
              actions={
                <>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={pl.enabled ? t('plugins.disablePlugin') : t('plugins.enablePlugin')}
                    onClick={() => togglePlugin.mutate(pl.id)}
                  >
                    {pl.enabled ? (
                      <IconPlugOff className="h-4 w-4" />
                    ) : (
                      <IconPower className="h-4 w-4" />
                    )}
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={t('plugins.uninstallPlugin')}
                    onClick={() => uninstallPlugin.mutate(pl.id)}
                  >
                    <IconTrash className="h-4 w-4 text-destructive" />
                  </Button>
                </>
              }
            />
          ))}
        </div>
      )}

      <Dialog open={installOpen} onOpenChange={setInstallOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('plugins.dialogTitle')}</DialogTitle>
            <DialogDescription>
              {t('plugins.dialogDescription')}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div>
              <Label className="mb-2">{t('plugins.nameLabel')}</Label>
              <Input
                type="text"
                value={plName}
                onChange={(e) => setPlName(e.target.value)}
                placeholder={t('plugins.namePlaceholder')}
              />
            </div>
            <div>
              <Label className="mb-2">{t('plugins.versionLabel')}</Label>
              <Input
                type="text"
                value={plVersion}
                onChange={(e) => setPlVersion(e.target.value)}
                placeholder={t('plugins.versionPlaceholder')}
              />
            </div>
            <div>
              <Label className="mb-2">{t('plugins.sourceLabel')}</Label>
              <Input
                type="text"
                value={plSource}
                onChange={(e) => setPlSource(e.target.value)}
                placeholder={t('plugins.sourcePlaceholder')}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setInstallOpen(false)}>
              {tc('actions.cancel')}
            </Button>
            <Button
              onClick={handleInstall}
              disabled={installPlugin.isPending || !plName.trim() || !plSource.trim()}
            >
              {installPlugin.isPending ? t('plugins.installing') : t('plugins.install')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
