// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconMail, IconBrandWindows, IconBrandGoogle } from '@tabler/icons-react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { useCreateEmailServer } from '../hooks/useEmailServers';
import { SMTPForm, type SMTPFormData } from './SMTPForm';
import { ExchangeForm, type ExchangeFormData } from './ExchangeForm';
import { GmailForm, type GmailFormData } from './GmailForm';

type CreateEmailServerDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

type ServerType = 'smtp' | 'exchange' | 'gmail' | null;

const DEFAULT_SMTP: SMTPFormData = {
  name: '',
  host: '',
  port: 587,
  encryption: 'starttls',
  authMethod: 'plain',
  username: '',
  password: '',
  fromAddress: '',
  fromName: '',
};

const DEFAULT_EXCHANGE: ExchangeFormData = {
  name: '',
  clientId: '',
  clientSecret: '',
  tenantId: '',
  fromAddress: '',
  fromName: '',
};

const DEFAULT_GMAIL: GmailFormData = {
  name: '',
  clientId: '',
  clientSecret: '',
  fromAddress: '',
  fromName: '',
};

export function CreateEmailServerDialog({ open, onOpenChange }: CreateEmailServerDialogProps) {
  const { t } = useTranslation('settings');
  const [step, setStep] = useState<'type' | 'config'>('type');
  const [serverType, setServerType] = useState<ServerType>(null);
  const [smtpData, setSmtpData] = useState(DEFAULT_SMTP);
  const [exchangeData, setExchangeData] = useState(DEFAULT_EXCHANGE);
  const [gmailData, setGmailData] = useState(DEFAULT_GMAIL);
  const createServer = useCreateEmailServer();

  function reset() {
    setStep('type');
    setServerType(null);
    setSmtpData(DEFAULT_SMTP);
    setExchangeData(DEFAULT_EXCHANGE);
    setGmailData(DEFAULT_GMAIL);
  }

  function handleOpenChange(value: boolean) {
    if (!value) reset();
    onOpenChange(value);
  }

  function handleSelectType(type: ServerType) {
    setServerType(type);
    setStep('config');
  }

  function handleBack() {
    setStep('type');
  }

  function handleCreate() {
    if (serverType === 'smtp') {
      createServer.mutate({
        name: smtpData.name,
        serverType: 'smtp',
        host: smtpData.host,
        port: smtpData.port,
        encryption: smtpData.encryption,
        authMethod: smtpData.authMethod,
        username: smtpData.username,
        password: smtpData.password,
        fromAddress: smtpData.fromAddress,
        fromName: smtpData.fromName,
      }, { onSuccess: () => handleOpenChange(false) });
    } else if (serverType === 'exchange') {
      createServer.mutate({
        name: exchangeData.name,
        serverType: 'exchange',
        clientId: exchangeData.clientId,
        clientSecret: exchangeData.clientSecret,
        tenantId: exchangeData.tenantId,
        fromAddress: exchangeData.fromAddress,
        fromName: exchangeData.fromName,
      }, { onSuccess: () => handleOpenChange(false) });
    } else if (serverType === 'gmail') {
      createServer.mutate({
        name: gmailData.name,
        serverType: 'gmail',
        clientId: gmailData.clientId,
        clientSecret: gmailData.clientSecret,
        fromAddress: gmailData.fromAddress,
        fromName: gmailData.fromName,
      }, { onSuccess: () => handleOpenChange(false) });
    }
  }

  const isValid =
    serverType === 'smtp'
      ? smtpData.name && smtpData.host && smtpData.port && smtpData.fromAddress
      : serverType === 'exchange'
        ? exchangeData.name && exchangeData.clientId && exchangeData.clientSecret && exchangeData.tenantId && exchangeData.fromAddress
        : serverType === 'gmail'
          ? gmailData.name && gmailData.clientId && gmailData.clientSecret && gmailData.fromAddress
          : false;

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>
            {step === 'type' ? t('email.selectType') : t('email.configuration')}
          </DialogTitle>
          <DialogDescription>
            {step === 'type'
              ? t('email.selectTypeDescription')
              : serverType === 'smtp'
                ? t('email.typeSMTP')
                : serverType === 'exchange'
                  ? t('email.typeExchange')
                  : t('email.typeGmail')}
          </DialogDescription>
        </DialogHeader>

        <div>
          {step === 'type' && (
            <div className="grid grid-cols-3 gap-3">
              <Button
                variant="outline"
                onClick={() => handleSelectType('smtp')}
                className="flex h-auto flex-col items-center gap-3 p-6"
              >
                <IconMail className="size-10 text-primary" />
                <span className="text-sm font-medium text-foreground">
                  {t('email.typeSMTP')}
                </span>
              </Button>
              <Button
                variant="outline"
                onClick={() => handleSelectType('exchange')}
                className="flex h-auto flex-col items-center gap-3 p-6"
              >
                <IconBrandWindows className="size-10 text-[#0078D4]" />
                <span className="text-sm font-medium text-foreground">
                  {t('email.typeExchange')}
                </span>
              </Button>
              <Button
                variant="outline"
                onClick={() => handleSelectType('gmail')}
                className="flex h-auto flex-col items-center gap-3 p-6"
              >
                <IconBrandGoogle className="size-10 text-[#4285F4]" />
                <span className="text-sm font-medium text-foreground">
                  {t('email.typeGmail')}
                </span>
              </Button>
            </div>
          )}

          {step === 'config' && serverType === 'smtp' && (
            <SMTPForm data={smtpData} onChange={setSmtpData} />
          )}

          {step === 'config' && serverType === 'exchange' && (
            <ExchangeForm data={exchangeData} onChange={setExchangeData} />
          )}

          {step === 'config' && serverType === 'gmail' && (
            <GmailForm data={gmailData} onChange={setGmailData} />
          )}
        </div>

        {step === 'config' && (
          <DialogFooter>
            <Button variant="outline" onClick={handleBack}>
              {t('common:back', 'Back')}
            </Button>
            <Button
              onClick={handleCreate}
              disabled={!isValid || createServer.isPending}
            >
              {createServer.isPending ? '...' : t('email.addServer')}
            </Button>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  );
}
