// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useAIProviderConnections, useDefaultAIProviderConnection } from '@resources/hooks/useAIProviderConnections';

export function useAIProviderPicker() {
  const { t } = useTranslation(['ai', 'common']);
  const { data: connections = [] } = useAIProviderConnections();
  const { data: defaultConnection } = useDefaultAIProviderConnection();
  // Empty means "use the estate default", which is what the backend does
  // with an empty ConnectionID — so the picker starts on the default
  // without having to wait for the connections query to resolve.
  const [connectionId, setConnectionId] = useState('');

  const enabledConnections = useMemo(() => connections.filter((c) => c.enabled), [connections]);

  // The default row is named rather than left implicit, so the operator
  // can see *which* provider "default" currently means without opening
  // settings — the whole reason a one-provider estate does not need this
  // control but a two-provider one does.
  const connectionOptions = useMemo(() => {
    const fallback = enabledConnections.find((c) => c.isDefault);
    return [
      {
        value: '',
        label: fallback
          ? t('aiAssistant.providerDefaultNamed', { name: fallback.name })
          : t('aiAssistant.providerDefault'),
      },
      ...enabledConnections.map((c) => ({
        value: c.id,
        label: c.defaultModel ? `${c.name} · ${c.defaultModel}` : c.name,
      })),
    ];
  }, [enabledConnections, t]);

  // A connection that was disabled or deleted while the dialog sat open
  // would otherwise leave the picker showing a row the backend will
  // reject. Fall back to the default instead of failing the next send.
  if (connectionId && !enabledConnections.some((c) => c.id === connectionId)) {
    setConnectionId('');
  }

  return { connectionId, setConnectionId, enabledConnections, connectionOptions, defaultConnection };
}

export type AIProviderPicker = ReturnType<typeof useAIProviderPicker>;
