// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IconStar,
  IconStarFilled,
  IconTrash,
  IconPencil,
  IconPlugConnected,
  IconLock,
  IconLogin2,
} from '@tabler/icons-react';
import { aiProviderIconNode } from './ai-provider-icons';
import type { AIAdapterSpec } from '../hooks/useAIProviderAdapters';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { Switch } from '@resources/components/ui/Switch';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import {
  useUpdateAIProviderConnection,
  useDeleteAIProviderConnection,
  useSetDefaultAIProviderConnection,
  type AIProviderConnection,
} from '../hooks/useAIProviderConnections';
import { PERM } from '@core/auth/permissionKeys';

type AIProviderConnectionCardProps = {
  connection: AIProviderConnection;
  /** Display name for the provider type (e.g. "OpenAI"). Sourced
   *  from the runtime adapter catalog. Falls back to the raw
   *  ProviderType when no catalog entry is loaded yet. */
  providerDisplayName: string;
  /** Runtime adapter spec for this connection's provider type, used
   *  to render the plugin's own brand icon. Undefined when the
   *  catalog hasn't loaded or the plugin has been uninstalled — the
   *  card then falls back to IconBolt. */
  providerSpec?: AIAdapterSpec;
  onEdit: (connection: AIProviderConnection) => void;
  onTest: (connection: AIProviderConnection) => void;
};

/** Renders one row of `ai_provider_connections` — a named LLM
 *  provider instance. The card keeps host-only concerns: enable
 *  toggle, set-default, test, edit, delete. Plugin-decorated config
 *  fields (api_key / base_url / default_model) are owned by the
 *  Edit dialog; this card only renders the summary. */
// The backend accepts settings.manage OR ai_assistant.manage on every
// /ai-providers route (ai_providers/routes.go). Gating the controls on
// settings.manage alone made the UI stricter than the check behind it: a
// user holding only ai_assistant.manage saw everything disabled.
const AI_PROVIDER_MANAGE = [PERM.SETTINGS_MANAGE, PERM.AI_ASSISTANT_MANAGE];
const CARD_ICON_CLASS = 'size-8 shrink-0 text-primary';

export function AIProviderConnectionCard({
  connection,
  providerDisplayName,
  providerSpec,
  onEdit,
  onTest,
}: AIProviderConnectionCardProps) {
  const { t } = useTranslation('settings');
  const [deleteOpen, setDeleteOpen] = useState(false);
  const updateConnection = useUpdateAIProviderConnection();
  const deleteConnection = useDeleteAIProviderConnection();
  const setDefault = useSetDefaultAIProviderConnection();

  function handleToggle(enabled: boolean) {
    updateConnection.mutate({ id: connection.id, enabled });
  }

  function handleSetDefault() {
    setDefault.mutate(connection.id);
  }

  function handleDelete() {
    deleteConnection.mutate(connection.id, {
      onSuccess: () => setDeleteOpen(false),
    });
  }

  return (
    <>
      <div className="mc-glass flex flex-col gap-4 rounded-lg p-4 sm:flex-row sm:items-center">
        {aiProviderIconNode(providerSpec?.Slug, providerSpec?.Icon, CARD_ICON_CLASS)}

        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h4 className="truncate text-sm font-medium text-foreground">{connection.name}</h4>
            {connection.isDefault && <Badge variant="default">{t('aiProviders.default')}</Badge>}
            <Badge variant={connection.enabled ? 'default' : 'secondary'}>
              {connection.enabled ? t('aiProviders.enabled') : t('aiProviders.disabled')}
            </Badge>
            {connection.authMethod === 'oauth' ? (
              // A sign-in connection with no token looks configured but
              // cannot call anything, so the card says which of the two
              // states it is in rather than showing a credential badge
              // that would be a lie.
              <Badge variant={connection.oauth?.connected ? 'outline' : 'secondary'}>
                <IconLogin2 className="mr-1 size-3" />
                {connection.oauth?.connected ? t('aiProviders.auth.signedIn') : t('aiProviders.auth.signInRequired')}
              </Badge>
            ) : (
              connection.hasApiKey && (
                <Badge variant="outline">
                  <IconLock className="mr-1 size-3" />
                  {t('aiProviders.apiKeySet')}
                </Badge>
              )
            )}
          </div>
          <p className="mt-0.5 text-xs break-all text-muted-foreground">
            {providerDisplayName}
            {connection.defaultModel ? ` · ${connection.defaultModel}` : ''}
            {connection.baseUrl ? ` · ${connection.baseUrl}` : ''}
          </p>
        </div>

        <div className="flex items-center gap-1 self-end sm:self-auto">
          <Button
            permission={AI_PROVIDER_MANAGE}
            variant="ghost"
            size="icon-sm"
            aria-label={connection.isDefault ? t('aiProviders.isDefault') : t('aiProviders.setDefault')}
            onClick={handleSetDefault}
            disabled={connection.isDefault || setDefault.isPending}
          >
            {connection.isDefault ? (
              <IconStarFilled className="size-4 text-warning" />
            ) : (
              <IconStar className="size-4" />
            )}
          </Button>
          <Switch
            checked={connection.enabled}
            onCheckedChange={handleToggle}
            disabled={updateConnection.isPending}
            aria-label={t('aiProviders.fields.enabled')}
          />
          <Button
            permission={AI_PROVIDER_MANAGE}
            variant="ghost"
            size="icon-sm"
            aria-label={t('aiProviders.testConnection.button')}
            onClick={() => onTest(connection)}
          >
            <IconPlugConnected className="size-4" />
          </Button>
          <Button
            permission={AI_PROVIDER_MANAGE}
            variant="ghost"
            size="icon-sm"
            aria-label={t('aiProviders.editProvider')}
            onClick={() => onEdit(connection)}
          >
            <IconPencil className="size-4" />
          </Button>
          <Button
            permission={AI_PROVIDER_MANAGE}
            variant="ghost"
            size="icon-sm"
            aria-label={t('aiProviders.deleteProvider')}
            onClick={() => setDeleteOpen(true)}
          >
            <IconTrash className="size-4 text-destructive" />
          </Button>
        </div>
      </div>

      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title={t('aiProviders.confirmDeleteTitle')}
        description={t('aiProviders.confirmDeleteDescription')}
        onConfirm={handleDelete}
        loading={deleteConnection.isPending}
        variant="destructive"
      />
    </>
  );
}
