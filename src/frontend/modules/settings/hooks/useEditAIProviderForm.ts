// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import {
  useUpdateAIProviderConnection,
  type AIProviderConnection,
  type AIProviderAuthMethod,
  type AIProviderOAuthInput,
} from './useAIProviderConnections';

/**
 * Edit-form state for one saved AI provider connection.
 *
 * Two fields carry "blank means keep" semantics - the API key and the OAuth
 * client secret are never returned by the backend, so an untouched field must
 * not overwrite the stored value. Extracted from the dialog so that rule lives
 * in one place rather than inside a component's render.
 */
export function useEditAIProviderForm(
  connection: AIProviderConnection,
  isOpen: boolean,
  onOpenChange: (isOpen: boolean) => void,
) {
  const updateConnection = useUpdateAIProviderConnection();

  const [itemName, setName] = useState(connection.name);
  const [baseUrl, setBaseUrl] = useState(connection.baseUrl ?? '');
  const [defaultModel, setDefaultModel] = useState(connection.defaultModel ?? '');
  // Empty string = "don't touch the existing API key". The backend
  // treats *string == nil OR empty as "leave the column as-is".
  const [apiKey, setApiKey] = useState('');
  const [enabled, setEnabled] = useState(connection.enabled);
  const [isDefault, setIsDefault] = useState(connection.isDefault);
  const [authMethod, setAuthMethod] = useState<AIProviderAuthMethod>(connection.authMethod);
  // The stored client secret never comes back, so the field starts
  // blank and blank means "keep it" — same rule as the API key.
  const [oauth, setOAuth] = useState<AIProviderOAuthInput>(storedOAuth(connection));

  const [seeded, setSeeded] = useState<{ isOpen: boolean; connection: typeof connection | null }>({
    isOpen: false,
    connection: null,
  });
  if (seeded.isOpen !== isOpen || seeded.connection !== connection) {
    setSeeded({ isOpen, connection });
    if (isOpen) seedFromConnection();
  }

  function seedFromConnection() {
    setName(connection.name);
    setBaseUrl(connection.baseUrl ?? '');
    setDefaultModel(connection.defaultModel ?? '');
    setApiKey('');
    setEnabled(connection.enabled);
    setIsDefault(connection.isDefault);
    setAuthMethod(connection.authMethod);
    setOAuth(storedOAuth(connection));
  }

  function handleSave() {
    const input: Parameters<typeof updateConnection.mutate>[0] = {
      id: connection.id,
      name: itemName.trim(),
      enabled,
      isDefault,
      baseUrl: baseUrl.trim() || undefined,
      defaultModel: defaultModel.trim() || undefined,
    };
    // Only send the api_key when the admin actually entered one;
    // the backend uses "leave blank to keep" semantics so we
    // forward an empty string as the explicit "don't touch" signal.
    if (apiKey.trim()) {
      input.apiKey = apiKey.trim();
    }
    input.authMethod = authMethod;
    // The endpoint config travels either way: an operator who flips
    // back to API-key auth should not lose the URLs they typed.
    input.oauth = oauth;
    updateConnection.mutate(input, {
      onSuccess: () => onOpenChange(false),
    });
  }

  const isValid = !!itemName.trim();
  const isPending = updateConnection.isPending;

  return {
    name: itemName,
    setName,
    baseUrl,
    setBaseUrl,
    defaultModel,
    setDefaultModel,
    apiKey,
    setApiKey,
    enabled,
    setEnabled,
    isDefault,
    setIsDefault,
    authMethod,
    setAuthMethod,
    oauth,
    setOAuth,
    handleSave,
    isValid,
    isPending,
  };
}

function storedOAuth(connection: AIProviderConnection): AIProviderOAuthInput {
  return {
    clientId: connection.oauth?.clientId ?? '',
    clientSecret: '',
    deviceUrl: connection.oauth?.deviceUrl ?? '',
    tokenUrl: connection.oauth?.tokenUrl ?? '',
    scopes: connection.oauth?.scopes ?? '',
  };
}
