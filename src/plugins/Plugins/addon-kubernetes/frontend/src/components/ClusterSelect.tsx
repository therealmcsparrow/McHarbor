// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { Select } from '@resources/components/ui/Select';
import { PluginLink } from '../nav-context';
import { KUBE_PERM } from '../permissions';
import { setSelectedCluster, useSelectedCluster } from '../cluster-store';
import { useClusters } from '../hooks/useClusters';

/**
 * Cluster picker for the Kubernetes pages.
 *
 * The topbar environment switcher is Docker-only — every host page under it
 * talks to the Docker API — so Kubernetes keeps its own selection. What
 * changed with the move out of core is where both halves live: the list comes
 * from the plugin's own `/api/kubernetes/clusters`, not the host's
 * `/environments`, and the selection sits in the plugin's own store rather
 * than on the host's environment store.
 *
 * With no cluster configured this renders a link to the plugin's Clusters page
 * rather than an empty dropdown, since an empty select gives the reader
 * nothing to act on.
 */
export function ClusterSelect() {
  const { t } = useTranslation('kubernetes');
  const selected = useSelectedCluster();
  const clustersQuery = useClusters();

  const clusters = useMemo(
    () => (clustersQuery.data ?? []).filter((cluster) => cluster.isActive),
    [clustersQuery.data],
  );

  // Keep the selection pointing at something real: a persisted id survives the
  // cluster being deleted, and a fresh install has no id at all.
  useEffect(() => {
    if (clusters.length === 0) return;
    if (clusters.some((cluster) => cluster.id === selected)) return;
    const preferred = clusters.find((cluster) => cluster.isDefault) ?? clusters[0];
    if (preferred) setSelectedCluster(preferred.id);
  }, [clusters, selected]);

  if (clustersQuery.isLoading) return null;

  if (clusters.length === 0) {
    return (
      <Button variant="outline" size="sm" permission={KUBE_PERM.CLUSTERS_MANAGE} asChild>
        <PluginLink to="clusters">{t('cluster.add')}</PluginLink>
      </Button>
    );
  }

  return (
    <Select
      variant="outline"
      value={selected}
      onChange={setSelectedCluster}
      ariaLabel={t('cluster.label')}
      placeholder={t('cluster.label')}
      className="min-w-[180px]"
      options={clusters.map((cluster) => ({ value: cluster.id, label: cluster.name }))}
    />
  );
}
