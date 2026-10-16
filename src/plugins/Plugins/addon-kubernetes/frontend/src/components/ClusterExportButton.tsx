// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Export the cluster definitions.
//
// Uninstalling a plugin drops its `plugin_<slug>_*` tables, so uninstalling
// Kubernetes deletes every cluster connection. That is correct plugin hygiene —
// the alternative is orphaned rows nobody can reach — but a report is
// regenerable and a cluster list is not, so there needs to be a way out that
// is not "remember what you typed".
//
// What this exports is deliberately *not* a backup. Credentials never leave the
// server: `kubeconfig`, `bearerToken` and `caCert` are write-only on the API
// and absent from every response, so the file carries names, namespaces, server
// URLs and which credential *style* each cluster used. Re-importing means
// pasting the kubeconfigs again — which is the honest trade, because a file
// that did contain them would be a set of cluster-admin credentials sitting in
// the browser's downloads folder.

import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { IconDownload } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { toast } from 'sonner';
import type { Cluster } from '../hooks/useClusters';
import { KUBE_PERM } from '../permissions';

type ClusterExportButtonProps = {
  clusters: Cluster[];
};

/** Shape of the exported document. Versioned so a future importer can tell. */
type ClusterExport = {
  kind: 'mcharbor.kubernetes.clusters';
  version: 1;
  exportedAt: string;
  /** True for every entry: this format never carries secrets. */
  credentialsIncluded: false;
  clusters: {
    name: string;
    namespace: string;
    serverUrl: string;
    isDefault: boolean;
    isActive: boolean;
    /** Which credential the original used, so the reader knows what to supply. */
    credential: 'kubeconfig' | 'token' | 'none';
  }[];
};

export function ClusterExportButton({ clusters }: ClusterExportButtonProps) {
  const { t } = useTranslation('kubernetes');

  const handleExport = useCallback(() => {
    const doc: ClusterExport = {
      kind: 'mcharbor.kubernetes.clusters',
      version: 1,
      exportedAt: new Date().toISOString(),
      credentialsIncluded: false,
      clusters: clusters.map((cluster) => ({
        name: cluster.name,
        namespace: cluster.namespace,
        serverUrl: cluster.serverUrl,
        isDefault: cluster.isDefault,
        isActive: cluster.isActive,
        credential: cluster.hasKubeconfig ? 'kubeconfig' : cluster.hasToken ? 'token' : 'none',
      })),
    };

    const blob = new Blob([JSON.stringify(doc, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = `mcharbor-kubernetes-clusters-${new Date().toISOString().slice(0, 10)}.json`;
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
    // Revoking immediately can race the download in some browsers; a tick is
    // enough and the object is small.
    window.setTimeout(() => URL.revokeObjectURL(url), 1000);

    toast.success(t('clusters.toast.exported', { count: clusters.length }));
  }, [clusters, t]);

  return (
    <Button
      variant="outline"
      size="sm"
      permission={KUBE_PERM.CLUSTERS_MANAGE}
      onClick={handleExport}
      disabled={clusters.length === 0}
      title={t('clusters.actions.exportHint')}
    >
      <IconDownload className="size-4" />
      {t('clusters.actions.export')}
    </Button>
  );
}
