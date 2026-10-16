// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file:
//   containers:create.errors.imageOrNameRequired

import { useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { IconBrandDocker, IconCode, IconDeviceFloppy, IconLayoutGrid, IconTerminal2 } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@resources/components/ui/Tabs';
import { Spinner } from '@resources/components/ui/Spinner';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Box, Flex } from '@resources/components/ui/Layout';
import { useNetworks } from '@resources/hooks/useNetworks';
import { OperationProgressDialog } from '@resources/components/OperationProgressDialog';
import type { NetworkInfo } from '@core/types/docker';
import { CreateContainerFormTab } from './CreateContainerFormTab';
import { CreateContainerTextAreaTab } from './CreateContainerTextAreaTab';
import type { CreateContainerTab } from './createContainerFormHelpers';
import { useCreateContainerForm } from './useCreateContainerForm';
import { useCreateContainerSubmit } from './useCreateContainerSubmit';

type CreateContainerDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated?: (id: string, name: string) => void;
};

export function CreateContainerDialog({ open: isOpen, onOpenChange, onCreated }: CreateContainerDialogProps) {
  const { t } = useTranslation('containers');
  const form = useCreateContainerForm(isOpen);
  const imageInputRef = useRef<HTMLInputElement>(null);
  const { create, createProgress, restoreProgress, handleCreate, isBusy, canApply } = useCreateContainerSubmit(
    form,
    onOpenChange,
    onCreated,
  );
  const networksQuery = useNetworks();
  const existingNetworks = (networksQuery.data ?? []) as NetworkInfo[];

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent
        className="max-w-3xl p-0"
        onOpenAutoFocus={(evt) => {
          evt.preventDefault();
          imageInputRef.current?.focus();
        }}
      >
        <DialogHeader>
          <Flex align="center" gap={2}>
            <IconBrandDocker className="size-5 text-primary" />
            <DialogTitle>{t('create.title')}</DialogTitle>
          </Flex>
          <DialogDescription>{t('create.description')}</DialogDescription>
        </DialogHeader>

        <DialogBody className="px-5 pb-2">
          <Tabs
            value={form.tab}
            onValueChange={(v) => form.setTab(v as CreateContainerTab)}
            className="flex flex-col gap-3"
          >
            <TabsList className="grid w-full grid-cols-3">
              <TabsTrigger value="form" className="gap-1.5">
                <IconLayoutGrid className="size-4" />
                {t('create.tabForm')}
              </TabsTrigger>
              <TabsTrigger value="compose" className="gap-1.5">
                <IconCode className="size-4" />
                {t('create.tabCompose')}
              </TabsTrigger>
              <TabsTrigger value="run" className="gap-1.5">
                <IconTerminal2 className="size-4" />
                {t('create.tabRun')}
              </TabsTrigger>
            </TabsList>

            <TabsContent value="form" className="mt-0">
              <CreateContainerFormTab
                imageInputRef={imageInputRef}
                form={form}
                existingNetworks={existingNetworks}
                onNetworkCreated={(networkValue) => form.setNetwork({ kind: 'existing', name: networkValue.Name })}
              />
            </TabsContent>

            <TabsContent value="compose" className="mt-0">
              <CreateContainerTextAreaTab
                label={t('create.compose.label')}
                help={t('create.compose.help')}
                placeholder={t('create.compose.placeholder')}
                value={form.composeText}
                onChange={form.setComposeText}
                onApply={form.applyFromCompose}
                applyLabel={t('actions.apply', { ns: 'common' })}
                error={form.parseError}
                warnings={form.parseWarnings}
              />
            </TabsContent>

            <TabsContent value="run" className="mt-0">
              <CreateContainerTextAreaTab
                label={t('create.run.label')}
                help={t('create.run.help')}
                placeholder={t('create.run.placeholder')}
                value={form.runText}
                onChange={form.setRunText}
                onApply={form.applyFromRun}
                applyLabel={t('actions.apply', { ns: 'common' })}
                error={form.parseError}
                warnings={form.parseWarnings}
              />
            </TabsContent>
          </Tabs>

          {create.isError && (
            <Box className="mt-2 rounded-md border border-destructive/40 bg-destructive/10 p-2 text-xs text-destructive">
              {create.error instanceof Error ? create.error.message : t('create.errors.createFailed')}
            </Box>
          )}
        </DialogBody>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={isBusy}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button onClick={handleCreate} disabled={!canApply}>
            {isBusy ? <Spinner size="sm" className="mr-2" /> : <IconDeviceFloppy className="mr-2 size-4" />}
            {isBusy ? t('actions.saving', { ns: 'common' }) : t('create.title')}
          </Button>
        </DialogFooter>
      </DialogContent>
      <OperationProgressDialog state={createProgress.dialogState} onClose={createProgress.closeDialog} />
      <OperationProgressDialog state={restoreProgress.dialogState} onClose={restoreProgress.closeDialog} />
    </Dialog>
  );
}
