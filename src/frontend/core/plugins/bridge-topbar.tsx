// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState, type ReactElement } from 'react';
import { createPortal } from 'react-dom';
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconPlug } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { hasPermission, useCurrentUserPermissions } from '@core/auth/useCurrentUserPermissions';
import { usePluginTopbar, type PluginTopbarItem } from './loader';

const DEFAULT_LABEL_NS = 'common';

type MountedButton = { id: string; target: HTMLElement };

/**
 * PluginTopbarBridge mounts one DOM button per plugin-contributed
 * topbar item into the host's #header-slot or #header-right-slot.
 * Honors `visibility: "permission:<perm>"` by checking the current
 * user's permissions via the existing useCurrentUserPermissions hook.
 *
 * Renderless except for the portal-mounted buttons. Buttons navigate
 * to the plugin's path (defaults to /plugins/<slug>).
 */
export function PluginTopbarBridge(): ReactElement | null {
  const items = usePluginTopbar();
  const { data: perms } = useCurrentUserPermissions();
  const navigate = useNavigate();
  const { t } = useTranslation('common');
  const [mounted, setMounted] = useState<MountedButton[]>([]);

  useEffect(() => {
    const visible = items.filter((it: PluginTopbarItem) => {
      if (!it.visibility || it.visibility === 'always') return true;
      if (it.visibility.startsWith('permission:')) {
        const perm = it.visibility.slice('permission:'.length);
        return hasPermission(perms, perm);
      }
      return false;
    });

    const next: MountedButton[] = [];
    for (const it of visible) {
      const slot = it.slot === 'right' ? '#header-right-slot' : '#header-slot';
      const host = document.querySelector<HTMLElement>(slot);
      if (!host) continue;
      const target = document.createElement('div');
      target.dataset['pluginTopbarId'] = `${it.plugin_slug}-${it.label_key}`;
      host.appendChild(target);
      next.push({ id: target.dataset['pluginTopbarId'] ?? '', target });
    }
    // eslint-disable-next-line react/set-state-in-effect -- mounts plugin buttons into header slots that only exist in the live DOM, so the portal targets cannot be derived during render
    setMounted(next);
    return () => {
      for (const m of next) m.target.remove();
    };
  }, [items, perms]);

  if (mounted.length === 0) return null;

  return (
    <>
      {mounted.map((m) => {
        const item = items.find((it: PluginTopbarItem) => `${it.plugin_slug}-${it.label_key}` === m.id);
        if (!item) return null;
        const labelNs = item.label_ns ?? DEFAULT_LABEL_NS;
        const label = t(`${labelNs}.${item.label_key}`, {
          // eslint-disable-next-line mcharbor/no-restricted-syntax -- i18n-open-set: plugin topbar labels are keyed by the plugin manifest, which may ship no bundle
          defaultValue: item.label_key,
        });
        const dest = item.path && item.path.length > 0 ? item.path : `/plugins/${item.plugin_slug}`;
        return createPortal(
          <Button variant="ghost" size="icon" aria-label={label} onClick={() => navigate(dest)}>
            <IconPlug className="size-4" />
          </Button>,
          m.target,
        );
      })}
    </>
  );
}
