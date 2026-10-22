// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type WhatsNewItem = {
  emoji?: string;
  titleKey: string;
  descriptionKey: string;
};

export function getReleaseItems(version: string): WhatsNewItem[] {
  if (version === '2026.10.02') {
    return [
      {
        titleKey: 'whatsNew.2026_10_02.diskAlerts.title',
        descriptionKey: 'whatsNew.2026_10_02.diskAlerts.description',
      },
      {
        titleKey: 'whatsNew.2026_10_02.diskStatus.title',
        descriptionKey: 'whatsNew.2026_10_02.diskStatus.description',
      },
      {
        titleKey: 'whatsNew.2026_10_02.tempCleanup.title',
        descriptionKey: 'whatsNew.2026_10_02.tempCleanup.description',
      },
    ];
  }
  if (version === '2026.10.01') {
    return [
      {
        titleKey: 'whatsNew.2026_10_01.selfProtection.title',
        descriptionKey: 'whatsNew.2026_10_01.selfProtection.description',
      },
      {
        titleKey: 'whatsNew.2026_10_01.rbacEnvScope.title',
        descriptionKey: 'whatsNew.2026_10_01.rbacEnvScope.description',
      },
      {
        titleKey: 'whatsNew.2026_10_01.setupToken.title',
        descriptionKey: 'whatsNew.2026_10_01.setupToken.description',
      },
      {
        titleKey: 'whatsNew.2026_10_01.unsignedPlugins.title',
        descriptionKey: 'whatsNew.2026_10_01.unsignedPlugins.description',
      },
      {
        titleKey: 'whatsNew.2026_10_01.pluginCatalog.title',
        descriptionKey: 'whatsNew.2026_10_01.pluginCatalog.description',
      },
      {
        titleKey: 'whatsNew.2026_10_01.agentReleases.title',
        descriptionKey: 'whatsNew.2026_10_01.agentReleases.description',
      },
      {
        titleKey: 'whatsNew.2026_10_01.gitopsWebhookSecret.title',
        descriptionKey: 'whatsNew.2026_10_01.gitopsWebhookSecret.description',
      },
      { titleKey: 'whatsNew.2026_10_01.csp.title', descriptionKey: 'whatsNew.2026_10_01.csp.description' },
      { titleKey: 'whatsNew.2026_10_01.compute.title', descriptionKey: 'whatsNew.2026_10_01.compute.description' },
      { titleKey: 'whatsNew.2026_10_01.plugins.title', descriptionKey: 'whatsNew.2026_10_01.plugins.description' },
      { titleKey: 'whatsNew.2026_10_01.database.title', descriptionKey: 'whatsNew.2026_10_01.database.description' },
      { titleKey: 'whatsNew.2026_10_01.ha.title', descriptionKey: 'whatsNew.2026_10_01.ha.description' },
      {
        titleKey: 'whatsNew.2026_10_01.instanceBackup.title',
        descriptionKey: 'whatsNew.2026_10_01.instanceBackup.description',
      },
      { titleKey: 'whatsNew.2026_10_01.rbac.title', descriptionKey: 'whatsNew.2026_10_01.rbac.description' },
      { titleKey: 'whatsNew.2026_10_01.secrets.title', descriptionKey: 'whatsNew.2026_10_01.secrets.description' },
      {
        titleKey: 'whatsNew.2026_10_01.credentialsAtRest.title',
        descriptionKey: 'whatsNew.2026_10_01.credentialsAtRest.description',
      },
      { titleKey: 'whatsNew.2026_10_01.tls.title', descriptionKey: 'whatsNew.2026_10_01.tls.description' },
      {
        titleKey: 'whatsNew.2026_10_01.hostUpdates.title',
        descriptionKey: 'whatsNew.2026_10_01.hostUpdates.description',
      },
      { titleKey: 'whatsNew.2026_10_01.reports.title', descriptionKey: 'whatsNew.2026_10_01.reports.description' },
      { titleKey: 'whatsNew.2026_10_01.incidents.title', descriptionKey: 'whatsNew.2026_10_01.incidents.description' },
      {
        titleKey: 'whatsNew.2026_10_01.paletteSearch.title',
        descriptionKey: 'whatsNew.2026_10_01.paletteSearch.description',
      },
      { titleKey: 'whatsNew.2026_10_01.hostIo.title', descriptionKey: 'whatsNew.2026_10_01.hostIo.description' },
      { titleKey: 'whatsNew.2026_10_01.vmHistory.title', descriptionKey: 'whatsNew.2026_10_01.vmHistory.description' },
      {
        titleKey: 'whatsNew.2026_10_01.restoreUploadSelection.title',
        descriptionKey: 'whatsNew.2026_10_01.restoreUploadSelection.description',
      },
      {
        titleKey: 'whatsNew.2026_10_01.vmVaultSecrets.title',
        descriptionKey: 'whatsNew.2026_10_01.vmVaultSecrets.description',
      },
      {
        titleKey: 'whatsNew.2026_10_01.containerRestore.title',
        descriptionKey: 'whatsNew.2026_10_01.containerRestore.description',
      },
      {
        titleKey: 'whatsNew.2026_10_01.accessibility.title',
        descriptionKey: 'whatsNew.2026_10_01.accessibility.description',
      },
      {
        titleKey: 'whatsNew.2026_10_01.outboundGuards.title',
        descriptionKey: 'whatsNew.2026_10_01.outboundGuards.description',
      },
    ];
  }
  if (version === '2.0.7') {
    return [
      { titleKey: 'whatsNew.2_0_7.firstRun.title', descriptionKey: 'whatsNew.2_0_7.firstRun.description' },
      { titleKey: 'whatsNew.2_0_7.help.title', descriptionKey: 'whatsNew.2_0_7.help.description' },
      { titleKey: 'whatsNew.2_0_7.shortcuts.title', descriptionKey: 'whatsNew.2_0_7.shortcuts.description' },
      { titleKey: 'whatsNew.2_0_7.skeletons.title', descriptionKey: 'whatsNew.2_0_7.skeletons.description' },
      { titleKey: 'whatsNew.2_0_7.bulk.title', descriptionKey: 'whatsNew.2_0_7.bulk.description' },
    ];
  }
  return [];
}
