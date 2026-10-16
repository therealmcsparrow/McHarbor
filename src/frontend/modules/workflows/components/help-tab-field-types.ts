// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { TFunction } from 'i18next';
import type { ConfigFieldType } from '../types';
import { STORAGE_LOCATION_SELECT } from './storage-field-type';

export function describeFieldType(type: ConfigFieldType, t: TFunction<'common'>): string {
  switch (type) {
    case 'textarea':
      return t('workflows.fieldTypeTextarea');
    case 'number':
      return t('workflows.fieldTypeNumber');
    case 'select':
      return t('workflows.fieldTypeSelect');
    case 'toggle':
      return t('workflows.fieldTypeToggle');
    case 'json':
      return t('workflows.fieldTypeJson');
    case 'key-value':
      return t('workflows.fieldTypeKeyValue');
    case 'expression':
      return t('workflows.fieldTypeExpression');
    case 'cron':
      return t('workflows.fieldTypeCron');
    case 'container-select':
      return t('workflows.fieldTypeContainerSelect');
    case 'environment-select':
      return t('workflows.fieldTypeEnvironmentSelect');
    case 'metric-conditions':
      return t('workflows.fieldTypeMetricConditions');
    case 'code':
      return t('workflows.fieldTypeCode');
    case 'link-output-select':
      return t('workflows.fieldTypeLinkOutputSelect');
    case 'email-server-select':
      return t('workflows.fieldTypeEmailServerSelect');
    case 'communication-channel-select':
      return t('workflows.fieldTypeCommunicationChannelSelect');
    case 'webhook-select':
      return t('workflows.fieldTypeWebhookSelect');
    case 'registry-select':
      return t('workflows.fieldTypeRegistrySelect');
    case STORAGE_LOCATION_SELECT:
      return t('workflows.fieldTypeStorageLocationSelect');
    case 'stack-select':
      return t('workflows.fieldTypeStackSelect');
    case 'image-select':
      return t('workflows.fieldTypeImageSelect');
    case 'plugin-select':
      return t('workflows.fieldTypePluginSelect');
    case 'image-scanner-select':
      return t('workflows.fieldTypeScannerSelect');
    case 'volume-select':
      return t('workflows.fieldTypeVolumeSelect');
    case 'network-select':
      return t('workflows.fieldTypeNetworkSelect');
    case 'backup-plan-select':
      return t('workflows.fieldTypeBackupPlanSelect');
    case 'backup-run-select':
      return t('workflows.fieldTypeBackupRunSelect');
    case 'multiselect':
      return t('workflows.fieldTypeMultiselect');
    case 'timezone-select':
      return t('workflows.fieldTypeTimezoneSelect');
    case 'container-mount-multiselect':
      return t('workflows.fieldTypeContainerMountMultiselect');
    case 'backup-restore-item-multiselect':
      return t('workflows.fieldTypeBackupRestoreItemMultiselect');
    case 'stack-service-select':
      return t('workflows.fieldTypeStackServiceSelect');
    case 'text':
    default:
      return t('workflows.fieldTypeText');
  }
}
