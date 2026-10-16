// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Switch } from '@resources/components/ui/Switch';
import { type AIProviderConnection } from '../hooks/useAIProviderConnections';
import { useEditAIProviderForm } from '../hooks/useEditAIProviderForm';
import { AIProviderAuthFields } from './AIProviderAuthFields';
import { AIProviderOAuthPanel } from './AIProviderOAuthPanel';
import { ProviderFormOutlet } from '@core/plugins/ProviderFormOutlet';
import { PERM } from '@core/auth/permissionKeys';

type EditAIProviderConnectionDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  connection: AIProviderConnection;
};

// Every /ai-providers route accepts settings.manage OR
// ai_assistant.manage, so the dialog's controls honour both instead of
// disabling themselves for a user the API would have let through.
const AI_PROVIDER_MANAGE = [PERM.SETTINGS_MANAGE, PERM.AI_ASSISTANT_MANAGE];

/** Edit an existing AI provider connection. Provider type is
 *  immutable — once a row is bound to a plugin type, the only
 *  way to change it is to delete the row and create a new one
 *  (avoids surprising the chat composer mid-session).
 *
 *  The credential half can be switched between a pasted API key and an
 *  OAuth sign-in at any time; switching to sign-in reveals the device
 *  flow panel for this saved row. */
export function EditAIProviderConnectionDialog({
  open: isOpen,
  onOpenChange,
  connection,
}: EditAIProviderConnectionDialogProps) {
  const { t } = useTranslation('settings');
  const {
    name: connectionName,
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
  } = useEditAIProviderForm(connection, isOpen, onOpenChange);

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>{t('aiProviders.editTitle', { name: connection.name })}</DialogTitle>
          <DialogDescription>{t('aiProviders.editDescription')}</DialogDescription>
        </DialogHeader>

        <DialogBody className="space-y-4">
          <div>
            <label htmlFor="edit-ai-provider-name" className="text-xs font-medium text-foreground">
              {t('aiProviders.fields.name')}
            </label>
            <Input
              id="edit-ai-provider-name"
              value={connectionName}
              onChange={(e) => setName(e.target.value)}
              placeholder={t('aiProviders.placeholders.name')}
              className="mt-1"
            />
          </div>
          <div>
            <label htmlFor="edit-ai-provider-base-url" className="text-xs font-medium text-foreground">
              {t('aiProviders.fields.apiBaseUrl')}
            </label>
            <Input
              id="edit-ai-provider-base-url"
              value={baseUrl}
              onChange={(e) => setBaseUrl(e.target.value)}
              placeholder={t('aiProviders.placeholders.apiBaseUrl')}
              className="mt-1"
            />
            <p className="mt-1 text-[11px] text-muted-foreground">{t('aiProviders.help.apiBaseUrl')}</p>
          </div>
          <div>
            <label htmlFor="edit-ai-provider-default-model" className="text-xs font-medium text-foreground">
              {t('aiProviders.fields.defaultModel')}
            </label>
            <Input
              id="edit-ai-provider-default-model"
              value={defaultModel}
              onChange={(e) => setDefaultModel(e.target.value)}
              placeholder={t('aiProviders.placeholders.defaultModel')}
              className="mt-1"
            />
          </div>
          <AIProviderAuthFields
            value={{ authMethod, apiKey, oauth }}
            onChange={(patch) => {
              if (patch.authMethod) setAuthMethod(patch.authMethod);
              if (patch.apiKey !== undefined) setApiKey(patch.apiKey);
              if (patch.oauth) setOAuth(patch.oauth);
            }}
            idPrefix="edit-ai-provider"
            apiKeyPlaceholder={connection.hasApiKey ? '••••••••' : t('aiProviders.placeholders.apiKey')}
            apiKeyHelp={t('aiProviders.help.apiKeyEdit')}
            hasClientSecret={connection.oauth?.hasClientSecret}
          />
          {authMethod === 'oauth' && (
            <AIProviderOAuthPanel connectionId={connection.id} permission={AI_PROVIDER_MANAGE} />
          )}
          <div className="flex items-center justify-between rounded-md border border-border bg-background p-3">
            <p className="text-sm font-medium text-foreground">{t('aiProviders.fields.enabled')}</p>
            <Switch checked={enabled} onCheckedChange={setEnabled} aria-label={t('aiProviders.fields.enabled')} />
          </div>
          <div className="flex items-center justify-between rounded-md border border-border bg-background p-3">
            <p className="text-sm font-medium text-foreground">{t('aiProviders.fields.isDefault')}</p>
            <Switch checked={isDefault} onCheckedChange={setIsDefault} aria-label={t('aiProviders.fields.isDefault')} />
          </div>
          <ProviderFormOutlet resource="ai_provider" kind="setup_guide" keyName={connection.providerType} />
        </DialogBody>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('aiProviders.cancel')}
          </Button>
          <Button permission={AI_PROVIDER_MANAGE} onClick={handleSave} disabled={!isValid || isPending}>
            {isPending ? t('aiProviders.saving') : t('aiProviders.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** Pull the saved, non-secret OAuth config into an editable draft.
 *  The client secret is intentionally blank: the backend never returns
 *  it, and blank means "keep the stored one". */
