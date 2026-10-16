// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// KubernetesDetailHeader — the hero, built to `ContainerDetailHeader`'s and
// `NetworkDetailHeader`'s grammar.
//
// Before this, a pod detail opened with a lone "Back to pods" outline button
// and then a grey label/value list: nothing said what state the pod was in
// without reading down to the `status` row, and the four things you might
// actually want to do to it (logs, scale, restart, delete) existed only back
// on the table you had just left. Identity, state and actions now sit in a
// card at the top of the content, exactly as they do for a container, a
// network, a volume and a stack.
//
// The breadcrumb is rendered inline rather than through `HeaderBreadcrumb`.
// That component portals `Breadcrumb` into the topbar, and `Breadcrumb` builds
// its crumbs with react-router's `Link` — this bundle carries its own
// contextless react-router copy, so a `Link` here throws and blanks the plugin
// root. `PluginLink` navigates through the host's router instead, and the trail
// keeps the same shape and the same chevron.

import { useTranslation } from 'react-i18next';
import {
  IconArrowsVertical,
  IconBox,
  IconChevronRight,
  IconFileText,
  IconRotate,
  IconTopologyStar3,
  IconTrash,
} from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { HeaderActionButton } from '@resources/components/ui/HeaderActionButton';
import { StatusIndicator } from '@resources/components/ui/StatusIndicator';
import type { StatusTone } from '@resources/theme/status-palette';
import { list, resourcePath } from '../format';
import { PluginLink } from '../nav-context';
import { KUBE_PERM } from '../permissions';
import type { Row } from '../types';
import type { ActionsKind } from './KubernetesActionsCell';
import { stateTone } from './kubernetes-facets';

export type KubernetesDetailActions = {
  onScale: () => void;
  onRestart: () => void;
  onDelete: () => void;
  onLogs: () => void;
};

type KubernetesDetailHeaderProps = {
  kind: ActionsKind;
  name: string;
  namespace: string;
  data?: Row;
  actions: KubernetesDetailActions;
};

/**
 * The one fact the reader came for.
 *
 * A pod states it directly (`Running`, `CrashLoopBackOff`). A deployment does
 * not — its health is the `ready` fraction, so "3/3" is success and anything
 * short of it is a warning rather than a failure: a deployment mid-rollout is
 * working, not broken.
 */
function health(kind: ActionsKind, data: Row | undefined): { tone: StatusTone; label: string } {
  if (!data) return { tone: 'neutral', label: '—' };

  if (kind === 'pods') {
    const statusValue = list(data.status);
    return { tone: stateTone(statusValue), label: statusValue };
  }

  const ready = list(data.ready);
  const [current, desired] = ready.split('/');
  const complete = current === desired && current !== '0' && desired !== undefined;
  return { tone: complete ? 'success' : 'warning', label: ready };
}

export function KubernetesDetailHeader({
  kind,
  name: itemName,
  namespace,
  data,
  actions,
}: KubernetesDetailHeaderProps) {
  const { t } = useTranslation('kubernetes');
  const Icon = kind === 'pods' ? IconBox : IconTopologyStar3;
  const state = health(kind, data);
  const node = kind === 'pods' ? list(data?.node) : '';
  const age = list(data?.age);

  return (
    <div className="flex flex-1 flex-col gap-2.5">
      {/* Orientation first: where this page sits, and the two ways back. */}
      <nav
        aria-label={t('detail.breadcrumbLabel')}
        className="flex min-w-0 items-center gap-1 text-xs text-muted-foreground"
      >
        <PluginLink to={resourcePath('')} className="truncate transition-colors hover:text-foreground">
          {t('title')}
        </PluginLink>
        <IconChevronRight aria-hidden="true" className="size-3.5 shrink-0 opacity-60" />
        <PluginLink to={resourcePath(`/${kind}`)} className="truncate transition-colors hover:text-foreground">
          {t(`tabs.${kind}`)}
        </PluginLink>
        <IconChevronRight aria-hidden="true" className="size-3.5 shrink-0 opacity-60" />
        <span className="truncate text-foreground">{itemName}</span>
      </nav>

      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex min-w-0 items-center gap-3">
          <span
            aria-hidden="true"
            className="grid size-11 shrink-0 place-items-center rounded-xl bg-primary/12 ring-1 ring-primary/25 ring-inset"
          >
            <Icon className="size-6 text-primary" />
          </span>
          <div className="min-w-0">
            <h1 className="truncate font-display text-2xl leading-tight font-semibold tracking-tight text-foreground">
              {itemName}
            </h1>
            <div className="mt-1 flex flex-wrap items-center gap-1.5">
              <StatusIndicator tone={state.tone} label={state.label} size="sm" />
              <Badge variant="default" className="px-1.5 py-0 text-[10px]">
                {namespace}
              </Badge>
              {node && node !== '-' && (
                <Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
                  {node}
                </Badge>
              )}
              {age && age !== '-' && (
                <span className="rounded-md bg-muted/60 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
                  {age}
                </span>
              )}
            </div>
          </div>
        </div>
      </div>

      {/* Actions on their own row under the identity block — the facts and
          their badges read first, the controls follow. Same set, same order
          and same permissions as the row's actions cell, so learning one
          teaches the other. */}
      <div className="flex flex-wrap items-center gap-1.5">
        {kind === 'pods' ? (
          <HeaderActionButton
            tooltip={t('actions.logs')}
            permission={KUBE_PERM.LOGS}
            onClick={actions.onLogs}
            icon={<IconFileText className="size-3.5" />}
            className="text-info hover:border-info/30 hover:bg-info/10"
          />
        ) : (
          <>
            <HeaderActionButton
              tooltip={t('actions.scale')}
              permission={KUBE_PERM.MANAGE}
              onClick={actions.onScale}
              icon={<IconArrowsVertical className="size-3.5" />}
              className="text-info hover:border-info/30 hover:bg-info/10"
            />
            <HeaderActionButton
              tooltip={t('actions.restart')}
              permission={KUBE_PERM.MANAGE}
              onClick={actions.onRestart}
              icon={<IconRotate className="size-3.5" />}
              className="text-warning hover:border-warning/30 hover:bg-warning/10"
            />
          </>
        )}
        <div className="mx-0.5 h-5 w-px bg-border" />
        <HeaderActionButton
          tooltip={t('actions.delete')}
          permission={KUBE_PERM.DELETE}
          onClick={actions.onDelete}
          variant="destructive"
          icon={<IconTrash className="size-3.5" />}
        />
      </div>
    </div>
  );
}
