// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { aiAdapterQueryKey } from './useAIProviderAdapters';
import { useCreateAIProviderConnection } from './useAIProviderConnections';
import { useAIProviderOAuthDefaults } from './useAIProviderOAuth';
import { emptyOAuthDraft } from '../components/AIProviderAuthFields';
import { useAIProviderPicker, type ProviderType } from './useAIProviderPicker';
import type { AIProviderConnectionDraft } from '../components/AIProviderConnectionFields';

const emptyDraft: AIProviderConnectionDraft = {
  name: '',
  baseUrl: '',
  defaultModel: '',
  apiKey: '',
  authMethod: 'api_key',
  oauth: emptyOAuthDraft,
  isDefault: true,
};

/**
 * The three-step create flow behind CreateAIProviderConnectionDialog: pick a
 * provider type, fill the credential form, and - for OAuth only - sign in.
 *
 * Extracted from the dialog so the component is markup; the step machine and
 * the create mutation were the bulk of its length.
 */
export function useCreateAIProviderForm(onOpenChange: (open: boolean) => void) {
  const queryClient = useQueryClient();
  // 'signin' is only reached by OAuth connections: the token needs a
  // row to live in, so the device flow can only start once the
  // connection has been created.
  const [step, setStep] = useState<'type' | 'config' | 'signin'>('type');
  const [providerType, setProviderType] = useState<ProviderType | null>(null);
  const [draft, setDraft] = useState<AIProviderConnectionDraft>(emptyDraft);
  const [createdId, setCreatedId] = useState<string | null>(null);
  const createConnection = useCreateAIProviderConnection();
  const oauthDefaults = useAIProviderOAuthDefaults();

  const {
    adaptersQuery,
    installedPlugins,
    pickerEntries,
    enabledTypes,
    disabledTypes,
    hasDisabledPlugin,
    hasAnyAIPluginInstalled,
  } = useAIProviderPicker();

  function reset() {
    setStep('type');
    setProviderType(null);
    setDraft(emptyDraft);
    setCreatedId(null);
  }

  function handleOpenChange(value: boolean) {
    if (!value && createConnection.isPending) return;
    if (!value) reset();
    onOpenChange(value);
  }

  function handleOpen() {
    // Plugins register adapters at boot, not on install. Force a
    // fresh fetch every time the admin opens the dialog so newly
    // installed-but-not-yet-booted providers don't appear hidden.
    void queryClient.invalidateQueries({ queryKey: aiAdapterQueryKey });
  }

  function handleSelectType(type: ProviderType) {
    setProviderType(type);
    const defaults = oauthDefaults.data?.[type];
    // A plugin that ships a complete OAuth config gets zero-config
    // sign-in: leave the draft blank so nothing plugin-owned is copied
    // into the connection, and let the backend overlay the plugin's
    // client ID and endpoints at sign-in time (and pick up later plugin
    // updates). Otherwise prefill the published endpoints; the client ID
    // stays empty because the operator brings their own registered app.
    setDraft((d) => ({
      ...d,
      oauth:
        defaults && !defaults.pluginProvided
          ? {
              ...emptyOAuthDraft,
              clientId: defaults.clientId ?? '',
              deviceUrl: defaults.deviceUrl ?? '',
              tokenUrl: defaults.tokenUrl ?? '',
              scopes: defaults.scopes ?? '',
            }
          : emptyOAuthDraft,
    }));
    setStep('config');
  }

  function handleCreate() {
    if (!providerType) return;
    const isOAuth = draft.authMethod === 'oauth';
    createConnection.mutate(
      {
        name: draft.name.trim(),
        providerType,
        baseUrl: draft.baseUrl.trim() || undefined,
        defaultModel: draft.defaultModel.trim() || undefined,
        apiKey: isOAuth ? undefined : draft.apiKey.trim() || undefined,
        authMethod: draft.authMethod,
        oauth: isOAuth ? draft.oauth : undefined,
        isDefault: draft.isDefault,
        enabled: true,
      },
      {
        onSuccess: (connection) => {
          // An API-key connection is finished the moment it is saved.
          // An OAuth one is not: it still needs the sign-in that makes
          // it usable, so the dialog stays open on that step instead of
          // leaving a credential-less row behind.
          if (isOAuth && connection?.id) {
            setCreatedId(connection.id);
            setStep('signin');
            return;
          }
          reset();
          onOpenChange(false);
        },
      },
    );
  }

  const selectedEntry = pickerEntries.find((entry) => entry.type === providerType);
  // True when the selected provider's plugin ships a complete OAuth
  // config: the operator enters nothing and the config fields are
  // hidden, so the required-field check below is skipped.
  const oauthPreconfigured = !!(providerType && oauthDefaults.data?.[providerType]?.pluginProvided);
  // An OAuth connection cannot sign in without the three fields the
  // device flow posts, so the form asks for them before saving rather
  // than failing on the first Sign in click - unless the plugin
  // provides them, in which case there is nothing to ask for.
  const isValid =
    !!draft.name.trim() &&
    !!providerType &&
    (draft.authMethod !== 'oauth' ||
      oauthPreconfigured ||
      (!!draft.oauth.clientId.trim() && !!draft.oauth.deviceUrl.trim() && !!draft.oauth.tokenUrl.trim()));
  const isPending = createConnection.isPending;

  return {
    step,
    setStep,
    reset,
    providerType,
    draft,
    setDraft,
    createdId,
    createConnection,
    adaptersQuery,
    installedPlugins,
    pickerEntries,
    enabledTypes,
    disabledTypes,
    hasDisabledPlugin,
    hasAnyAIPluginInstalled,
    handleOpenChange,
    handleOpen,
    handleSelectType,
    handleCreate,
    selectedEntry,
    oauthPreconfigured,
    isValid,
    isPending,
  };
}
