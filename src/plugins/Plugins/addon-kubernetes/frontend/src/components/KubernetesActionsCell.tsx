// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconArrowsVertical, IconFileText, IconRotate, IconTrash } from '@tabler/icons-react';
import { ActionButton } from '@resources/components/ui/ActionButton';
import type { ResourceRef } from '../hooks/useKubernetesActions';
import type { Row } from '../types';
import { KUBE_PERM } from '../permissions';

/** Which action set a row gets. Only these two tables have write actions. */
export type ActionsKind = 'pods' | 'deployments';

type KubernetesActionsCellProps = {
  row: Row;
  kind: ActionsKind;
  onScale: (ref: ResourceRef) => void;
  onRestart: (ref: ResourceRef) => void;
  onDelete: (ref: ResourceRef, kind: ActionsKind) => void;
  onLogs: (ref: ResourceRef) => void;
};

export function KubernetesActionsCell({ row, kind, onScale, onRestart, onDelete, onLogs }: KubernetesActionsCellProps) {
  const { t } = useTranslation('kubernetes');
  const namespace = typeof row.namespace === 'string' ? row.namespace : '';
  const itemName = typeof row.name === 'string' ? row.name : '';

  // Cluster-scoped rows and anything missing an identity cannot be addressed by
  // the namespaced endpoints, so they get no actions rather than broken ones.
  if (!namespace || !itemName) return null;
  const ref: ResourceRef = { namespace, name: itemName };

  return (
    <div className="flex items-center justify-end">
      {kind === 'pods' && (
        <ActionButton
          label={t('actions.logs')}
          permission={KUBE_PERM.LOGS}
          onClick={() => onLogs(ref)}
          icon={<IconFileText className="h-3.5 w-3.5" />}
        />
      )}
      {kind === 'deployments' && (
        <>
          <ActionButton
            label={t('actions.scale')}
            permission={KUBE_PERM.MANAGE}
            onClick={() => onScale(ref)}
            icon={<IconArrowsVertical className="h-3.5 w-3.5" />}
          />
          <ActionButton
            label={t('actions.restart')}
            permission={KUBE_PERM.MANAGE}
            onClick={() => onRestart(ref)}
            icon={<IconRotate className="h-3.5 w-3.5" />}
          />
        </>
      )}
      <ActionButton
        label={t('actions.delete')}
        permission={KUBE_PERM.DELETE}
        onClick={() => onDelete(ref, kind)}
        icon={<IconTrash className="h-3.5 w-3.5 text-destructive" />}
      />
    </div>
  );
}
