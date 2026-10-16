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
import { PERM } from '@core/auth/permissionKeys';
import { AIProviderOAuthPanel } from './AIProviderOAuthPanel';
import { useCreateAIProviderForm } from '../hooks/useCreateAIProviderForm';
import { AIProviderTypePicker } from './AIProviderTypePicker';
import { AIProviderConnectionFields } from './AIProviderConnectionFields';

type CreateAIProviderConnectionDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

const AI_PROVIDER_MANAGE = [PERM.SETTINGS_MANAGE, PERM.AI_ASSISTANT_MANAGE];

export function CreateAIProviderConnectionDialog({
  open: isOpen,
  onOpenChange,
}: CreateAIProviderConnectionDialogProps) {
  const { t } = useTranslation('settings');
  const {
    step,
    setStep,
    reset,
    providerType,
    draft,
    setDraft,
    createdId,
    adaptersQuery,
    installedPlugins,
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
  } = useCreateAIProviderForm(onOpenChange);

  return (
    <Dialog
      open={isOpen}
      onOpenChange={(v) => {
        if (v) handleOpen();
        handleOpenChange(v);
      }}
    >
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>
            {step === 'type'
              ? t('aiProviders.selectType')
              : step === 'signin'
                ? t('aiProviders.auth.signInTitle')
                : t('aiProviders.configuration')}
          </DialogTitle>
          <DialogDescription>
            {step === 'signin'
              ? t('aiProviders.auth.signInDescription')
              : step === 'type'
                ? enabledTypes.length > 0
                  ? t('aiProviders.selectTypeDescriptionAvailable')
                  : hasDisabledPlugin
                    ? t('aiProviders.pluginsDisabledDescription')
                    : t('aiProviders.selectTypeDescription')
                : selectedEntry
                  ? `${selectedEntry.displayName} (${selectedEntry.type})`
                  : ''}
          </DialogDescription>
        </DialogHeader>

        <DialogBody>
          {step === 'type' && (
            <AIProviderTypePicker
              enabledTypes={enabledTypes}
              disabledTypes={disabledTypes}
              hasAnyAIPluginInstalled={hasAnyAIPluginInstalled}
              loading={adaptersQuery.isLoading && installedPlugins.length === 0}
              error={adaptersQuery.isError ? adaptersQuery.error : null}
              onRetry={() => void adaptersQuery.refetch()}
              onSelect={handleSelectType}
            />
          )}

          {step === 'config' && providerType && (
            <AIProviderConnectionFields
              value={draft}
              onChange={(patch) => setDraft((d) => ({ ...d, ...patch }))}
              oauthPreconfigured={oauthPreconfigured}
            />
          )}

          {step === 'signin' && <AIProviderOAuthPanel connectionId={createdId} permission={AI_PROVIDER_MANAGE} />}
        </DialogBody>

        {step === 'config' && (
          <DialogFooter>
            <Button variant="outline" onClick={() => setStep('type')}>
              {t('common:actions.back')}
            </Button>
            <Button permission={AI_PROVIDER_MANAGE} onClick={handleCreate} disabled={!isValid || isPending}>
              {isPending ? t('aiProviders.saving') : t('aiProviders.addProvider')}
            </Button>
          </DialogFooter>
        )}

        {step === 'signin' && (
          <DialogFooter>
            <Button
              onClick={() => {
                reset();
                onOpenChange(false);
              }}
            >
              {t('aiProviders.auth.done')}
            </Button>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  );
}
