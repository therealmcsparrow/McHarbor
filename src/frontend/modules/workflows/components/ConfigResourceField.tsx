// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { STORAGE_LOCATION_SELECT } from './storage-field-type';
import { ConfigFieldLabelled } from './ConfigFieldLabelled';
import { EnvironmentSelect } from './EnvironmentSelect';
import { ContainerSelect } from './ContainerSelect';
import { LinkOutputSelect } from './LinkOutputSelect';
import { CommunicationChannelSelect } from './CommunicationChannelSelect';
import { EmailChannelSelect } from './EmailChannelSelect';
import { WebhookSelect } from './WebhookSelect';
import { RegistrySelect } from './RegistrySelect';
import { StorageLocationSelect } from './StorageLocationSelect';
import { StackSelect } from './StackSelect';
import { ImageSelect } from './ImageSelect';
import { PluginSelect } from './PluginSelect';
import { ScannerSelect } from './ScannerSelect';
import { VolumeSelect } from './VolumeSelect';
import { NetworkSelect } from './NetworkSelect';
import { BackupPlanSelect } from './BackupPlanSelect';
import { BackupRunSelect } from './BackupRunSelect';
import { MultiSelectField } from './MultiSelectField';
import { TimezoneSelect } from './TimezoneSelect';
import { ContainerMountMultiSelect } from './ContainerMountMultiSelect';
import { BackupRestoreItemMultiSelect } from './BackupRestoreItemMultiSelect';
import { StackServiceSelect } from './StackServiceSelect';
import type { ConfigField } from '../types';

interface ConfigResourceFieldProps {
  field: ConfigField;
  value: unknown;
  onChange: (v: unknown) => void;
  nodeConfig: Record<string, unknown>;
  nodeKey?: string;
  fieldLabel: string;
  strVal: string;
  envId?: string;
  containerId?: string;
}

export function ConfigResourceField({
  field,
  value,
  onChange,
  nodeConfig,
  nodeKey,
  fieldLabel,
  strVal,
  envId,
  containerId,
}: ConfigResourceFieldProps) {
  const set = (v: string) => onChange(v);
  const nonEmpty = (v: unknown) => (typeof v === 'string' && v ? v : undefined);
  const labelled = (control: ReactNode) => (
    <ConfigFieldLabelled label={fieldLabel} required={field.required}>
      {control}
    </ConfigFieldLabelled>
  );

  switch (field.type) {
    case 'multiselect':
      return (
        <MultiSelectField field={field} value={value} onChange={onChange} nodeKey={nodeKey} fieldLabel={fieldLabel} />
      );

    case 'timezone-select':
      return labelled(<TimezoneSelect value={strVal} onChange={set} ariaLabel={fieldLabel} />);

    case 'container-mount-multiselect':
      return (
        <ContainerMountMultiSelect
          field={field}
          value={value}
          onChange={onChange}
          fieldLabel={fieldLabel}
          envId={envId}
          containerId={containerId}
        />
      );

    case 'backup-restore-item-multiselect':
      return (
        <BackupRestoreItemMultiSelect
          field={field}
          value={value}
          onChange={onChange}
          fieldLabel={fieldLabel}
          runId={nonEmpty(nodeConfig.run_id)}
          secretKey={typeof nodeConfig.secret_key === 'string' ? nodeConfig.secret_key : undefined}
        />
      );

    case 'stack-service-select':
      return labelled(
        <StackServiceSelect
          value={strVal}
          onChange={set}
          envId={envId}
          stackName={nonEmpty(nodeConfig.stack_name)}
          allowCustom={field.allowCustom}
        />,
      );

    case 'environment-select':
      return labelled(<EnvironmentSelect value={strVal} onChange={set} />);

    case 'container-select':
      return labelled(
        <ContainerSelect
          value={strVal}
          onChange={set}
          envId={typeof nodeConfig.environment === 'string' ? nodeConfig.environment : undefined}
        />,
      );

    case 'email-server-select':
      return labelled(<EmailChannelSelect value={strVal} onChange={set} />);

    case 'communication-channel-select':
      return labelled(
        <CommunicationChannelSelect
          value={strVal}
          onChange={set}
          channelType={field.channelType}
          incidentOnly={field.incidentOnly}
        />,
      );

    case 'webhook-select':
      return labelled(<WebhookSelect value={strVal} onChange={set} />);

    case 'registry-select':
      return labelled(<RegistrySelect value={strVal} onChange={set} />);

    case STORAGE_LOCATION_SELECT:
      return labelled(<StorageLocationSelect value={strVal} onChange={set} />);

    case 'stack-select':
      return labelled(<StackSelect value={strVal} onChange={set} envId={envId} allowCustom={field.allowCustom} />);

    case 'image-select':
      return labelled(<ImageSelect value={strVal} onChange={set} envId={envId} allowCustom={field.allowCustom} />);

    case 'plugin-select':
      return labelled(
        <PluginSelect
          value={strVal}
          onChange={set}
          source={field.source}
          envId={envId}
          allowCustom={field.allowCustom}
        />,
      );

    case 'image-scanner-select':
      return labelled(<ScannerSelect value={strVal} onChange={set} />);

    case 'volume-select':
      return labelled(<VolumeSelect value={strVal} onChange={set} envId={envId} />);

    case 'network-select':
      return labelled(<NetworkSelect value={strVal} onChange={set} envId={envId} />);

    case 'backup-plan-select':
      return labelled(<BackupPlanSelect value={strVal} onChange={set} envId={envId} containerId={containerId} />);

    case 'backup-run-select':
      return labelled(
        <BackupRunSelect
          value={strVal}
          onChange={set}
          envId={envId}
          containerId={containerId}
          completedOnly={nodeConfig.action !== 'cancel_run' && nodeConfig.action !== 'get_run'}
        />,
      );

    case 'link-output-select':
      return labelled(<LinkOutputSelect value={strVal} onChange={set} />);

    default:
      return null;
  }
}
