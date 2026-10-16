// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import {
  useCreateProvider,
  type GroupMapping,
  type IdentityProvider as IdentityProviderValue,
} from '../../hooks/useIdentityProviders';
import type { MappingConfig } from '../../hooks/useIdentitySetup';

export type WizardStep = 'type' | 'configure' | 'access' | 'review' | 'done';

export const WIZARD_STEPS: WizardStep[] = ['type', 'configure', 'access', 'review', 'done'];

const EMPTY: Record<string, string> = {};

export function useProviderWizard() {
  const [step, setStep] = useState<WizardStep>('type');
  const [providerType, setProviderType] = useState<string | null>(null);
  const [config, setConfig] = useState<Record<string, string>>(EMPTY);
  const [autoProvision, setAutoProvision] = useState(true);
  const [defaultRoleId, setDefaultRoleId] = useState('');
  const [groupMappingEnabled, setGroupMappingEnabled] = useState(false);
  const [groupMappings, setGroupMappings] = useState<GroupMapping[]>([]);
  const [autoImportGroups, setAutoImportGroups] = useState(false);
  const [created, setCreated] = useState<IdentityProviderValue | null>(null);
  const create = useCreateProvider();

  const reset = () => {
    setStep('type');
    setProviderType(null);
    setConfig(EMPTY);
    setAutoProvision(true);
    setDefaultRoleId('');
    setGroupMappingEnabled(false);
    setGroupMappings([]);
    setAutoImportGroups(false);
    setCreated(null);
    create.reset();
  };

  const selectType = (type: string) => {
    setProviderType(type);
    setConfig(EMPTY);
    setStep('configure');
  };

  const mapping: MappingConfig = {
    groupMappingEnabled,
    groupMappings: groupMappings.filter((m) => m.providerGroup && m.mcharborGroupId),
    autoImportGroups,
    autoProvision,
    defaultRoleId: defaultRoleId || undefined,
  };

  const submit = () => {
    if (!providerType) return;
    const { name: itemName, ...pluginConfig } = config;
    create.mutate(
      {
        name: itemName ?? '',
        providerType,
        config: pluginConfig,
        autoProvision,
        defaultRoleId: defaultRoleId || undefined,
        groupMappingEnabled,
        groupMappings: mapping.groupMappings,
        autoImportGroups,
      },
      {
        onSuccess: (provider) => {
          setCreated(provider);
          setStep('done');
        },
      },
    );
  };

  return {
    step,
    setStep,
    providerType,
    selectType,
    config,
    setConfig,
    prefill: (values: Record<string, string>) => setConfig((current) => ({ ...current, ...values })),
    autoProvision,
    setAutoProvision,
    defaultRoleId,
    setDefaultRoleId,
    groupMappingEnabled,
    setGroupMappingEnabled,
    groupMappings,
    setGroupMappings,
    autoImportGroups,
    setAutoImportGroups,
    mapping,
    created,
    create,
    submit,
    reset,
    nameValid: Boolean(config.name?.trim()),
  };
}

export type ProviderWizard = ReturnType<typeof useProviderWizard>;
