// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconExternalLink, IconCheck, IconDownload } from '@tabler/icons-react';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@resources/components/ui/Dialog';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { AppInstallationsList } from './AppInstallations';
import type { AppTemplate } from '../types';

interface AppDetailDialogProps {
  app: AppTemplate | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onInstall: (app: AppTemplate) => void;
}

export function AppDetailDialog({ app, open, onOpenChange, onInstall }: AppDetailDialogProps) {
  const { t } = useTranslation('common');
  if (!app) return null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg p-0">
        <DialogHeader>
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 shrink-0 items-center justify-center overflow-hidden rounded-md bg-muted">
              {app.logo ? (
                <img
                  src={app.logo}
                  alt={app.name}
                  className="h-8 w-8 object-contain"
                  onError={(e) => {
                    e.currentTarget.style.display = 'none';
                  }}
                />
              ) : (
                <span className="text-lg font-bold text-muted-foreground">
                  {app.name.charAt(0)}
                </span>
              )}
            </div>
            <div>
              <DialogTitle>{app.name}</DialogTitle>
              <DialogDescription className="mt-0.5 flex flex-wrap items-center gap-2">
                <Badge variant="secondary">{app.category}</Badge>
                {app.version && (
                  <Badge variant="outline">{app.version}</Badge>
                )}
              </DialogDescription>
            </div>
          </div>
        </DialogHeader>

        <DialogBody className="space-y-4 p-4">
          <p className="text-sm text-muted-foreground">{app.description}</p>

          <div>
            <h4 className="mb-1 text-xs font-medium text-foreground">{t('appStore.dockerImage')}</h4>
            <code className="text-xs text-muted-foreground">{app.image}</code>
          </div>

          {app.ports.length > 0 && (
            <div>
              <h4 className="mb-1 text-xs font-medium text-foreground">{t('appStore.ports')}</h4>
              <div className="space-y-1">
                {app.ports.map((p) => (
                  <div key={`${p.host}-${p.container}-${p.protocol}`} className="text-xs text-muted-foreground">
                    {p.host}:{p.container}/{p.protocol || 'tcp'}
                  </div>
                ))}
              </div>
            </div>
          )}

          {app.volumes.length > 0 && (
            <div>
              <h4 className="mb-1 text-xs font-medium text-foreground">{t('appStore.volumes')}</h4>
              <div className="space-y-1">
                {app.volumes.map((v) => (
                  <div key={`${v.host}-${v.container}-${v.readOnly ? 'ro' : 'rw'}`} className="text-xs text-muted-foreground">
                    {v.host} &rarr; {v.container}
                    {v.readOnly && ' (ro)'}
                  </div>
                ))}
              </div>
            </div>
          )}

          {app.envVars.length > 0 && (
            <div>
              <h4 className="mb-1 text-xs font-medium text-foreground">{t('appStore.envVars')}</h4>
              <div className="space-y-1">
                {app.envVars.map((ev) => (
                  <div key={ev.key} className="flex items-baseline gap-2 text-xs">
                    <code className="font-mono text-foreground">{ev.key}</code>
                    <span className="text-muted-foreground">{ev.description}</span>
                  </div>
                ))}
              </div>
            </div>
          )}

          {app.minMemory && (
            <div className="text-xs text-muted-foreground">
              {t('appStore.minMemory')} <span className="font-medium text-foreground">{app.minMemory}</span>
            </div>
          )}

          <AppInstallationsList installations={app.installations ?? []} />

        </DialogBody>
        <DialogFooter className="justify-between">
          <div className="flex items-center gap-2">
            {app.website && (
              <a
                href={app.website}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1 text-xs text-primary hover:underline"
              >
                {t('appStore.website')} <IconExternalLink className="size-3" />
              </a>
            )}
            {app.docsUrl && (
              <a
                href={app.docsUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1 text-xs text-primary hover:underline"
              >
                {t('appStore.docs')} <IconExternalLink className="size-3" />
              </a>
            )}
          </div>
          <div className="flex items-center gap-2">
            {app.installed && (
              <Badge variant="success" className="gap-1">
                <IconCheck className="size-3" />
                {t('appStore.installed')}
              </Badge>
            )}
            <Button
              size="sm"
              className="gap-1"
              onClick={() => {
                onInstall(app);
                onOpenChange(false);
              }}
            >
              <IconDownload className="size-3.5" />
              {t('appStore.install')}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

