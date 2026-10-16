// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { StatusError } from '@resources/components/ui/StatusError';
import { Badge } from '@resources/components/ui/Badge';
import type { PluginEntry } from '../types/plugins';
import { PluginIcon } from './plugin-icons';
import { parseAuthor } from './PluginSignature';
import { PluginCardActions } from './PluginCardActions';
import { versionLabel } from '../utils/versionLabel';

type PluginCardProps = {
  plugin: PluginEntry & { installed?: boolean };
  // Opens the install modal in picker mode. The PluginInstallDialog
  // owns the version selector + confirm step, then calls back into
  // the parent's stream.startInstall() with the chosen URL. We don't
  // pick a version here — the modal is the source of truth so the
  // card stays a passive preview.
  onInstall: (plugin: PluginEntry) => void;
  // Surfaced on the catalog card so the user does not have to navigate
  // to a separate "Installed" page just to uninstall. The store page
  // wires this to useUninstallPlugin + the confirm dialog.
  onUninstall?: (slug: string) => void;
  onViewDetail?: (plugin: PluginEntry) => void;
  // Update path: hits /api/appstore/plugins/{slug}/update/stream.
  // The handler tears down the existing installation and reinstalls
  // at the catalog version, so the on-disk .so + appstore_plugins
  // row both advance in one shot. Wired by the parent page from
  // useStreamPluginAction().startUpdate. Optional so the card
  // gracefully falls back to onInstall when the host doesn't
  // expose the update endpoint. No version arg — the host always
  // picks the latest catalog entry by slug for updates today.
  onUpdate?: (plugin: PluginEntry) => void;
  hasUpdate?: boolean;
  // The version the user has on disk. Lets the card show
  // "v1.0.4 (installed v1.0.2)" so the version mismatch is visible
  // without opening the detail dialog. Parent (PluginCategoryGrid)
  // forwards this from useInstalledPlugins().
  installedVersion?: string;
  installedUnsigned?: boolean;
  installedEnabled?: boolean;
  needsRepair?: boolean;
  builtFor?: string;
  repairError?: string;
};

export function PluginCard({
  plugin,
  onInstall,
  onUninstall,
  onViewDetail,
  onUpdate,
  hasUpdate,
  installedVersion,
  installedUnsigned,
  installedEnabled,
  needsRepair,
  builtFor,
  repairError,
}: PluginCardProps) {
  const { t } = useTranslation(['common', 'plugins']);

  // Authors are written `Name <mail@example.com>`: the card shows the
  // name, and the address — when there is one — becomes the link target
  // rather than visible text.
  const { name: creator, email: creatorEmail } = parseAuthor(plugin.author ?? '');

  function handleInstall() {
    onInstall(plugin);
  }
  function handleUpdate() {
    const handler = onUpdate ?? onInstall;
    handler(plugin);
  }

  return (
    <div className="mc-glass flex h-full flex-col gap-3 rounded-lg p-4 transition-colors hover:border-primary/40">
      {/* Header: the icon sits beside a two-line identity block —
          name on top, version directly beneath it — so every card
          presents the same anchor regardless of name length. */}
      <div className="flex items-start gap-3">
        <div className="flex size-10 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
          <PluginIcon
            icon={plugin.icon}
            fromCatalog={Boolean(plugin.icon_url)}
            slug={plugin.slug}
            alt={plugin.name}
            className="size-7"
          />
        </div>
        <div className="min-w-0 flex-1">
          <h3 className="truncate text-sm font-medium text-foreground">{plugin.name}</h3>
          <p className="mt-0.5 truncate font-mono text-xs text-muted-foreground">
            {versionLabel(plugin.version)}
            {plugin.installed && installedVersion && installedVersion !== plugin.version && (
              <span className="ml-1.5">({t('store.installedVersion', { version: installedVersion })})</span>
            )}
          </p>
        </div>
      </div>

      {/* Category sits on its own row between the header and the
          description, left-aligned with the text under it. */}
      {plugin.category && (
        <div>
          <Badge variant="outline">{plugin.category}</Badge>
        </div>
      )}

      {/* Four lines of description, reserved whether or not the text
          fills them: a grid of cards whose meta rows land at different
          heights reads as broken even when every card is correct. */}
      <p className="line-clamp-4 min-h-[4rem] text-xs leading-4 text-muted-foreground">{plugin.description}</p>

      {/* Meta: who made it, then what it is doing on this host — the
          status badges right-aligned above the actions. Pinned to the
          bottom of the flexible space so the action row underneath
          never floats. */}
      <div className="mt-auto space-y-1.5">
        {creator && (
          <p className="truncate text-xs text-muted-foreground">
            {t('card.createdBy', { ns: 'plugins' })}{' '}
            {creatorEmail ? (
              <a
                href={`mailto:${creatorEmail}`}
                className="rounded-sm font-medium text-primary hover:underline focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
              >
                {creator}
              </a>
            ) : (
              <span className="font-medium text-foreground">{creator}</span>
            )}
          </p>
        )}
        <div className="flex items-start justify-end gap-2">
          <div className="flex shrink-0 flex-wrap items-center justify-end gap-1.5">
            {hasUpdate && <Badge variant="destructive">{t('store.updateAvailable')}</Badge>}
            {plugin.installed && !needsRepair && <Badge variant="success">{t('store.installed')}</Badge>}
            {plugin.installed && installedUnsigned && (
              <Badge variant="warning">{t('unsigned.badge', { ns: 'plugins' })}</Badge>
            )}
            {needsRepair && (
              <>
                <Badge variant="destructive">{t('repair.needed', { ns: 'plugins' })}</Badge>
                <StatusError
                  error={repairError}
                  title={t('repair.failed', { ns: 'plugins' })}
                  description={t('repair.explanation', {
                    ns: 'plugins',
                    builtFor: builtFor ?? '',
                    current: installedVersion ?? '',
                  })}
                />
              </>
            )}
          </div>
        </div>
      </div>

      {/* Version selector lives in PluginInstallDialog, not here.
          The signature badge takes the left of this row: third-party
          plugins nobody signed install after a confirmation, and the state sits beside
          the button that would install one. */}
      <PluginCardActions
        plugin={plugin}
        installedUnsigned={installedUnsigned}
        installedEnabled={installedEnabled}
        needsRepair={needsRepair}
        hasUpdate={hasUpdate}
        onInstall={handleInstall}
        onUpdate={handleUpdate}
        onUninstall={onUninstall}
        onViewDetail={onViewDetail}
      />
    </div>
  );
}
