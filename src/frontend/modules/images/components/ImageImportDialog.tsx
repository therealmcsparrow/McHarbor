// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Input } from '@resources/components/ui/Input';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@resources/components/ui/Tabs';

type ImageImportDialogProps = {
  open: boolean;
  pullPending: boolean;
  importPending: boolean;
  onOpenChange: (open: boolean) => void;
  onPull: (imageName: string) => void;
  onImport: (file: File) => void;
};

export function ImageImportDialog({
  open,
  pullPending,
  importPending,
  onOpenChange,
  onPull,
  onImport,
}: ImageImportDialogProps) {
  const { t } = useTranslation('images');
  const { t: tc } = useTranslation('common');
  const [tab, setTab] = useState('registry');
  const [imageName, setImageName] = useState('');
  const [importFile, setImportFile] = useState<File | null>(null);

  function resetState(nextOpen: boolean) {
    onOpenChange(nextOpen);
    if (!nextOpen) {
      setImportFile(null);
      setImageName('');
      setTab('registry');
    }
  }

  function handlePull() {
    if (imageName.trim()) {
      onPull(imageName.trim());
    }
  }

  function handleImport() {
    if (importFile) {
      onImport(importFile);
    }
  }

  return (
    <Dialog open={open} onOpenChange={resetState}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('import.title')}</DialogTitle>
          <DialogDescription>{t('import.description')}</DialogDescription>
        </DialogHeader>
        <Tabs value={tab} onValueChange={setTab}>
          <TabsList className="w-full">
            <TabsTrigger value="registry" className="flex-1">
              {t('import.registryTab')}
            </TabsTrigger>
            <TabsTrigger value="file" className="flex-1">
              {t('import.fileTab')}
            </TabsTrigger>
          </TabsList>
          <TabsContent value="registry">
            <Input
              variant="outline"
              type="text"
              value={imageName}
              onChange={(event) => setImageName(event.target.value)}
              placeholder={t('pull.placeholder')}
              onKeyDown={(event) => event.key === 'Enter' && handlePull()}
            />
          </TabsContent>
          <TabsContent value="file">
            <label className="flex cursor-pointer items-center justify-center rounded-lg border-2 border-dashed border-border px-4 py-8 text-sm text-muted-foreground transition-colors hover:border-primary/50 hover:text-foreground">
              <input
                type="file"
                accept=".tar,.tar.gz,.tgz"
                className="hidden"
                onChange={(event) => setImportFile(event.target.files?.[0] ?? null)}
              />
              {importFile ? t('import.selectedFile', { name: importFile.name }) : t('import.selectFile')}
            </label>
          </TabsContent>
        </Tabs>
        <DialogFooter>
          <Button variant="outline" onClick={() => resetState(false)}>
            {tc('actions.cancel')}
          </Button>
          {tab === 'registry' ? (
            <Button onClick={handlePull} disabled={pullPending || !imageName.trim()}>
              {pullPending ? t('pull.pulling') : t('pull.submit')}
            </Button>
          ) : (
            <Button onClick={handleImport} disabled={importPending || !importFile}>
              {importPending ? t('import.importing') : t('import.submit')}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
