// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconHelpCircle } from '@tabler/icons-react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { PluginFormOutlet } from '@core/plugins/PluginFormOutlet';
import { SetupGuidePanel } from '@resources/components/SetupGuidePanel';
import { useCreateChannel, type ChannelType } from '../hooks/useNotificationChannels';
import { useNotificationAdapters } from '../hooks/useCommunicationAdapters';
import { communicationChannelOptionsWithPlugins, type CommunicationCreateType } from './communication-channel-options';
import { PERM } from '@core/auth/permissionKeys';

type CreateChannelDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function CreateChannelDialog({ open: isOpen, onOpenChange }: CreateChannelDialogProps) {
  const { t } = useTranslation('settings');
  const [step, setStep] = useState<'type' | 'config'>('type');
  const [communicationType, setCommunicationType] = useState<CommunicationCreateType | null>(null);
  const [helpOpen, setHelpOpen] = useState(false);
  const [pluginConfig, setPluginConfig] = useState<Record<string, string>>({});
  const [pluginErrors, setPluginErrors] = useState<Record<string, string>>({});
  const createChannel = useCreateChannel();
  const { data: pluginAdapters } = useNotificationAdapters();
  const mergedOptions = communicationChannelOptionsWithPlugins(pluginAdapters);

  function reset() {
    setStep('type');
    setCommunicationType(null);
    setHelpOpen(false);
    setPluginConfig({});
    setPluginErrors({});
  }

  function handleOpenChange(value: boolean) {
    if (!value) reset();
    onOpenChange(value);
  }

  function handleSelectType(type: CommunicationCreateType) {
    setCommunicationType(type);
    setPluginConfig({});
    setPluginErrors({});
    setStep('config');
  }

  function handleBack() {
    setHelpOpen(false);
    setStep('type');
  }

  function handleCreate() {
    if (!communicationType) return;

    const channelType = communicationType as ChannelType;
    const base = { channelType };

    const { name: channelName, ...config } = pluginConfig;

    createChannel.mutate({ ...base, name: channelName ?? '', config }, { onSuccess: () => handleOpenChange(false) });
  }

  const selectedOption = mergedOptions.find((opt) => opt.type === communicationType);
  const isValid = !!communicationType && !!pluginConfig.name && Object.keys(pluginErrors).length === 0;
  const isPending = createChannel.isPending;

  function handlePluginError(field: string, message: string | null) {
    setPluginErrors((current) => {
      const next = { ...current };
      if (message) {
        next[field] = message;
      } else {
        delete next[field];
      }
      return next;
    });
  }

  function renderGuidePanel(type: CommunicationCreateType) {
    return (
      <SetupGuidePanel>
        <PluginFormOutlet
          resource="channel"
          kind="setup_guide"
          keyName={type}
          value={pluginConfig}
          onChange={setPluginConfig}
          i18nPrefix="commSmtp"
        />
      </SetupGuidePanel>
    );
  }

  return (
    <Dialog open={isOpen} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>
            {step === 'type' ? t('communications.selectType') : t('communications.configuration')}
          </DialogTitle>
          <DialogDescription>
            {step === 'type'
              ? t('communications.selectTypeDescription')
              : selectedOption
                ? selectedOption.descriptionIsKey
                  ? t(selectedOption.description)
                  : selectedOption.description
                : ''}
          </DialogDescription>
        </DialogHeader>

        <div>
          {step === 'type' && (
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
              {mergedOptions.map((option) => (
                <Button
                  key={option.type}
                  variant="outline"
                  onClick={() => handleSelectType(option.type)}
                  className="flex h-auto flex-col items-center gap-3 p-4"
                >
                  <div className="flex size-8 items-center justify-center text-foreground">{option.icon}</div>
                  <span className="text-xs font-medium text-foreground">
                    {option.labelIsKey ? t(option.label) : option.label}
                  </span>
                </Button>
              ))}
            </div>
          )}

          {step === 'config' && communicationType && (
            <PluginFormOutlet
              resource="channel"
              kind="config_form"
              keyName={communicationType}
              value={pluginConfig}
              onChange={setPluginConfig}
              onError={handlePluginError}
              i18nPrefix="commSmtp"
            />
          )}

          {step === 'config' && helpOpen && communicationType && renderGuidePanel(communicationType)}
        </div>

        {step === 'config' && (
          <DialogFooter className="flex-col items-stretch gap-2 sm:flex-row sm:items-center sm:justify-between">
            <Button variant="ghost" onClick={() => setHelpOpen((current) => !current)} aria-expanded={helpOpen}>
              <IconHelpCircle className="size-4" />
              {helpOpen ? t('communications.hideSetupGuide') : t('communications.showSetupGuide')}
            </Button>
            <div className="flex flex-wrap items-center justify-end gap-2">
              <Button variant="outline" onClick={handleBack}>
                {t('common:actions.back')}
              </Button>
              <Button permission={PERM.COMMUNICATIONS_MANAGE} onClick={handleCreate} disabled={!isValid || isPending}>
                {isPending ? '...' : t('communications.addChannel')}
              </Button>
            </div>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  );
}
