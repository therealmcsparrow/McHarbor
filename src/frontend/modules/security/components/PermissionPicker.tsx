// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';

type PermissionPickerProps = {
  allPermissions: string[];
  selected: string[];
  onChange?: (permissions: string[]) => void;
  readOnly?: boolean;
};

type ResourceGroup = {
  resource: string;
  labelKey: string;
  permissions: { perm: string; action: string }[];
};

const ACTION_ORDER = ['view', 'manage', 'delete', 'access'];

export function PermissionPicker({ allPermissions, selected, onChange, readOnly }: PermissionPickerProps) {
  const { t } = useTranslation('security');

  const groups = useMemo(() => {
    const map = new Map<string, ResourceGroup>();

    for (const perm of allPermissions) {
      const dotIdx = perm.indexOf('.');
      if (dotIdx === -1) continue;

      const resource = perm.substring(0, dotIdx);
      const action = perm.substring(dotIdx + 1);

      if (!map.has(resource)) {
        // Map resource key to i18n key
        const labelKey = RESOURCE_LABELS[resource] ?? resource;
        map.set(resource, { resource, labelKey, permissions: [] });
      }
      const group = map.get(resource);
      if (group) {
        group.permissions.push({ perm, action });
      }
    }

    // Sort permissions within each group by ACTION_ORDER
    for (const group of map.values()) {
      group.permissions.sort((a, b) => {
        const ai = ACTION_ORDER.indexOf(a.action);
        const bi = ACTION_ORDER.indexOf(b.action);
        return (ai === -1 ? 99 : ai) - (bi === -1 ? 99 : bi);
      });
    }

    return Array.from(map.values());
  }, [allPermissions]);

  const toggle = (perm: string) => {
    if (readOnly || !onChange) return;
    if (selected.includes(perm)) {
      onChange(selected.filter((p) => p !== perm));
    } else {
      onChange([...selected, perm]);
    }
  };

  // Collect all unique actions across all groups
  const allActions = useMemo(() => {
    const set = new Set<string>();
    for (const g of groups) {
      for (const p of g.permissions) {
        set.add(p.action);
      }
    }
    return ACTION_ORDER.filter((a) => set.has(a));
  }, [groups]);

  const getGroupLabel = (group: ResourceGroup) =>
    t([`permissions.${group.labelKey}`, `permissions.${group.resource}`], {
      defaultValue: humanizeResource(group.resource),
    });

  return (
    <div className="rounded-lg border border-border">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-border bg-muted/30">
            <th className="px-3 py-2 text-left font-medium text-muted-foreground">{t('common:labels.resource')}</th>
            {allActions.map((action) => (
              <th key={action} className="px-3 py-2 text-center font-medium capitalize text-muted-foreground">
                {t(`roles.${action}`)}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {groups.map((group) => (
            <tr key={group.resource} className="border-b border-border last:border-0">
              <td className="px-3 py-2 font-medium">{getGroupLabel(group)}</td>
              {allActions.map((action) => {
                const match = group.permissions.find((p) => p.action === action);
                return (
                  <td key={action} className="px-3 py-2 text-center">
                    {match ? (
                      <input
                        type="checkbox"
                        checked={selected.includes(match.perm)}
                        onChange={() => toggle(match.perm)}
                        disabled={readOnly}
                        className="size-4 rounded border-border accent-primary disabled:opacity-50"
                      />
                    ) : (
                      <span className="text-muted-foreground/30">-</span>
                    )}
                  </td>
                );
              })}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function humanizeResource(resource: string) {
  return resource
    .split('_')
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ');
}

const RESOURCE_LABELS: Record<string, string> = {
  containers: 'containers',
  images: 'images',
  volumes: 'volumes',
  networks: 'networks',
  stacks: 'stacks',
  environments: 'environments',
  users: 'users',
  settings: 'settings',
  email_servers: 'emailServers',
  communications: 'communications',
  store_apps: 'storeApps',
  store_nodes: 'storeNodes',
  store_widgets: 'storeWidgets',
  terminal: 'terminal',
  logs: 'logs',
  api_keys: 'apiKeys',
  groups: 'groups',
  roles: 'roles',
  pods: 'pods',
  deployments: 'deployments',
  k8s_services: 'k8sServices',
  namespaces: 'namespaces',
};
