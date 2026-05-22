// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { useUpdateEmailServer, type EmailServer } from '../hooks/useEmailServers';
import { SMTPForm, type SMTPFormData } from './SMTPForm';
import { ExchangeForm, type ExchangeFormData } from './ExchangeForm';
import { GmailForm, type GmailFormData } from './GmailForm';

type EditEmailServerDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  server: EmailServer;
};

export function EditEmailServerDialog({ open, onOpenChange, server }: EditEmailServerDialogProps) {
  const { t } = useTranslation('settings');
  const updateServer = useUpdateEmailServer();

  const [smtpData, setSmtpData] = useState<SMTPFormData>({
    name: '',
    host: '',
    port: 587,
    encryption: 'starttls',
    authMethod: 'plain',
    username: '',
    password: '',
    fromAddress: '',
    fromName: '',
  });

  const [exchangeData, setExchangeData] = useState<ExchangeFormData>({
    name: '',
    clientId: '',
    clientSecret: '',
    tenantId: '',
    fromAddress: '',
    fromName: '',
  });

  const [gmailData, setGmailData] = useState<GmailFormData>({
    name: '',
    clientId: '',
    clientSecret: '',
    fromAddress: '',
    fromName: '',
  });

  useEffect(() => {
    if (!open) return;

    if (server.serverType === 'smtp') {
      setSmtpData({
        name: server.name,
        host: server.host ?? '',
        port: server.port ?? 587,
        encryption: server.encryption ?? 'starttls',
        authMethod: server.authMethod ?? 'plain',
        username: server.username ?? '',
        password: '',
        fromAddress: server.fromAddress,
        fromName: server.fromName ?? '',
      });
    } else if (server.serverType === 'exchange') {
      setExchangeData({
        name: server.name,
        clientId: server.clientId ?? '',
        clientSecret: '',
        tenantId: server.tenantId ?? '',
        fromAddress: server.fromAddress,
        fromName: server.fromName ?? '',
      });
    } else if (server.serverType === 'gmail') {
      setGmailData({
        name: server.name,
        clientId: server.clientId ?? '',
        clientSecret: '',
        fromAddress: server.fromAddress,
        fromName: server.fromName ?? '',
      });
    }
  }, [open, server]);

  function handleSave() {
    if (server.serverType === 'smtp') {
      updateServer.mutate({
        id: server.id,
        name: smtpData.name,
        host: smtpData.host,
        port: smtpData.port,
        encryption: smtpData.encryption,
        authMethod: smtpData.authMethod,
        username: smtpData.username,
        password: smtpData.password || undefined,
        fromAddress: smtpData.fromAddress,
        fromName: smtpData.fromName,
      }, { onSuccess: () => onOpenChange(false) });
    } else if (server.serverType === 'exchange') {
      updateServer.mutate({
        id: server.id,
        name: exchangeData.name,
        clientId: exchangeData.clientId,
        clientSecret: exchangeData.clientSecret || undefined,
        tenantId: exchangeData.tenantId,
        fromAddress: exchangeData.fromAddress,
        fromName: exchangeData.fromName,
      }, { onSuccess: () => onOpenChange(false) });
    } else if (server.serverType === 'gmail') {
      updateServer.mutate({
        id: server.id,
        name: gmailData.name,
        clientId: gmailData.clientId,
        clientSecret: gmailData.clientSecret || undefined,
        fromAddress: gmailData.fromAddress,
        fromName: gmailData.fromName,
      }, { onSuccess: () => onOpenChange(false) });
    }
  }

  const typeLabel =
    server.serverType === 'smtp'
      ? t('email.typeSMTP')
      : server.serverType === 'exchange'
        ? t('email.typeExchange')
        : t('email.typeGmail');

  const isValid =
    server.serverType === 'smtp'
      ? smtpData.name && smtpData.host && smtpData.port && smtpData.fromAddress
      : server.serverType === 'exchange'
        ? exchangeData.name && exchangeData.clientId && exchangeData.tenantId && exchangeData.fromAddress
        : server.serverType === 'gmail'
          ? gmailData.name && gmailData.clientId && gmailData.fromAddress
          : false;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>{t('email.editServer')}</DialogTitle>
          <DialogDescription>{typeLabel}</DialogDescription>
        </DialogHeader>

        <div>
          {server.serverType === 'smtp' && (
            <SMTPForm data={smtpData} onChange={setSmtpData} isEdit />
          )}

          {server.serverType === 'exchange' && (
            <ExchangeForm data={exchangeData} onChange={setExchangeData} isEdit />
          )}

          {server.serverType === 'gmail' && (
            <GmailForm data={gmailData} onChange={setGmailData} isEdit />
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('common:cancel', 'Cancel')}
          </Button>
          <Button
            onClick={handleSave}
            disabled={!isValid || updateServer.isPending}
          >
            {updateServer.isPending ? '...' : t('common:save', 'Save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
