// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconDownload, IconExternalLink } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Box, Flex } from '@resources/components/ui/Layout';
import { Switch } from '@resources/components/ui/Switch';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { useHostVersion } from '@resources/hooks/useHostVersion';
import { useUpdateChannels } from '@resources/hooks/useUpdateChannels';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { useSetPluginChannel } from '../hooks/usePlugins';
import { compareVersions, isPrerelease } from '@resources/utils/versionOrder';
import type { PluginEntry } from '../types/plugins';
import { PluginIcon } from './plugin-icons';
import { parseAuthor, PluginSignatureIcon } from './PluginSignature';
import { versionLabel } from '../utils/versionLabel';
import { PluginReturnDialog } from './PluginReturnDialog';

interface PluginDetailDialogProps {
  plugin: PluginEntry | null;
  open: boolean;
  installed: boolean;
  installedVersion?: string;
  onOpenChange: (open: boolean) => void;
  onInstall: (plugin: PluginEntry) => void;
}

// The override is one-way: effective setting = Store switch OR this plugin's own. With the
// Store switch on the row reads on and disabled, so the reason points at the Plugin Store.
function PrereleaseOverride({ plugin, installedVersion }: { plugin: PluginEntry; installedVersion?: string }) {
  const { t } = useTranslation('plugins');
  const base = useId();
  const canManage = usePermission(PERM.PLUGINS_MANAGE);
  const { settings } = useUpdateChannels();
  const save = useSetPluginChannel();
  const [saved, setSaved] = useState<boolean | null | undefined>(undefined);

  const globalOn = settings?.pluginPrereleases === true;
  const own = saved === undefined ? (plugin.prereleaseOverride ?? false) : (saved ?? false);
  const checked = globalOn || own;
  const locked = !canManage || globalOn || save.isPending;

  const newestStable = plugin.versions.find((v) => !isPrerelease(v.version))?.version ?? '';
  const hasNewerPrerelease = plugin.versions.some(
    (v) => isPrerelease(v.version) && compareVersions(v.version, newestStable) > 0,
  );
  const installedOnPrerelease = installedVersion !== undefined && isPrerelease(installedVersion);

  const reasons: { id: string; text: string }[] = [];
  if (!canManage) reasons.push({ id: 'need', text: t('channels.needs', { permission: PERM.PLUGINS_MANAGE }) });
  if (globalOn) reasons.push({ id: 'global', text: t('channels.override.globalOn') });
  if (checked && !hasNewerPrerelease) reasons.push({ id: 'none', text: t('channels.override.none') });
  if (installedOnPrerelease && !checked) reasons.push({ id: 'off', text: t('channels.override.followsOff') });
  const describedBy = [`${base}-hint`, ...reasons.map((r) => `${base}-${r.id}`)].join(' ');

  return (
    <div className="space-y-1">
      <label className="flex items-center gap-2 text-sm font-medium text-foreground">
        <Switch
          checked={checked}
          aria-disabled={locked}
          aria-busy={save.isPending}
          aria-describedby={describedBy}
          onCheckedChange={(next) => {
            if (locked) return;
            save.mutate(
              { slug: plugin.slug, prereleases: next },
              { onSuccess: (result) => setSaved(result.prereleases) },
            );
          }}
        />
        <span>{t('channels.override.label')}</span>
      </label>
      <p id={`${base}-hint`} className="text-xs text-muted-foreground">
        {t('channels.override.hint')}
      </p>
      {reasons.map((r) => (
        <p key={r.id} id={`${base}-${r.id}`} className="text-xs text-muted-foreground">
          {r.text}
        </p>
      ))}
    </div>
  );
}

export function PluginDetailDialog({
  plugin,
  open: isOpen,
  installed,
  installedVersion,
  onOpenChange,
  onInstall,
}: PluginDetailDialogProps) {
  const { t } = useTranslation(['common', 'plugins']);
  const [returning, setReturning] = useState(false);
  const hostReasonId = useId();
  const hostPrerelease = isPrerelease(useHostVersion());
  if (!plugin) return null;
  const onPrerelease = installed && installedVersion !== undefined && isPrerelease(installedVersion);

  // Same treatment as the card: the name shows, the address only ever
  // lives in the href.
  const author = parseAuthor(plugin.author ?? '');

  const homepage = /^https?:\/\//i.test(plugin.homepage) ? plugin.homepage : null;

  return (
    <>
      <Dialog open={isOpen} onOpenChange={onOpenChange}>
        <DialogContent className="max-w-lg p-0">
          <DialogHeader>
            <Flex align="start" gap={3}>
              <Box className="flex size-10 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
                <PluginIcon
                  icon={plugin.icon}
                  fromCatalog={Boolean(plugin.icon_url)}
                  slug={plugin.slug}
                  alt=""
                  className="size-6"
                />
              </Box>
              <Box className="flex-1">
                <DialogTitle>{plugin.name}</DialogTitle>
                <DialogDescription className="sr-only">{plugin.description}</DialogDescription>
              </Box>
            </Flex>
          </DialogHeader>

          <DialogBody className="space-y-4 p-4">
            <TypographyText tone="muted">{plugin.description}</TypographyText>
            <Flex wrap gap={2}>
              {plugin.category && <Badge variant="secondary">{plugin.category}</Badge>}
              <Badge variant="outline">{versionLabel(plugin.version)}</Badge>
              {author.name && (
                <Badge variant="outline">
                  {author.email ? (
                    <a
                      href={`mailto:${author.email}`}
                      className="rounded-sm text-primary hover:underline focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
                    >
                      {author.name}
                    </a>
                  ) : (
                    author.name
                  )}
                </Badge>
              )}
              {plugin.license && <Badge variant="outline">{plugin.license}</Badge>}
              <PluginSignatureIcon signed={!!plugin.signature} />
            </Flex>
            {homepage && (
              <a
                href={homepage}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1 rounded-sm text-sm text-primary hover:underline focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
              >
                {t('appStore.website')}
                <IconExternalLink className="size-3.5" />
              </a>
            )}
            {onPrerelease && hostPrerelease && (
              <p id={hostReasonId} className="text-xs text-muted-foreground">
                {t('return.prereleaseHost', { ns: 'plugins' })}
              </p>
            )}
            <PrereleaseOverride key={plugin.slug} plugin={plugin} installedVersion={installedVersion} />
            {plugin.versions.length > 0 && (
              <div>
                <h3 className="text-xs font-medium text-muted-foreground">{t('card.versions', { ns: 'plugins' })}</h3>
                <ul className="mt-1 space-y-1">
                  {plugin.versions.map((v) => (
                    <li key={v.version} className="flex items-center gap-2 font-mono text-xs text-foreground">
                      <span>{versionLabel(v.version)}</span>
                      {isPrerelease(v.version) && (
                        <Badge variant="outline">{t('card.prerelease', { ns: 'plugins' })}</Badge>
                      )}
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </DialogBody>

          <DialogFooter>
            {onPrerelease && <Badge variant="warning">{t('card.installedPrerelease', { ns: 'plugins' })}</Badge>}
            {onPrerelease && (
              <Button
                size="sm"
                variant="outline"
                className="aria-disabled:cursor-not-allowed aria-disabled:opacity-50"
                aria-disabled={hostPrerelease ? true : undefined}
                aria-describedby={hostPrerelease ? hostReasonId : undefined}
                onClick={() => {
                  if (!hostPrerelease) setReturning(true);
                }}
              >
                {t('return.open', { ns: 'plugins' })}
              </Button>
            )}
            {installed ? (
              <Badge variant="success">{t('catalog.installed', { ns: 'plugins' })}</Badge>
            ) : (
              <Button
                size="sm"
                className="focus-visible:ring-2 focus-visible:ring-ring"
                onClick={() => {
                  onInstall(plugin);
                  onOpenChange(false);
                }}
              >
                <IconDownload className="size-3.5" />
                {t('appStore.install')}
              </Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>
      {returning && onPrerelease && installedVersion !== undefined && (
        <PluginReturnDialog
          slug={plugin.slug}
          name={plugin.name}
          installedVersion={installedVersion}
          onOpenChange={(next) => {
            if (!next) setReturning(false);
          }}
        />
      )}
    </>
  );
}
