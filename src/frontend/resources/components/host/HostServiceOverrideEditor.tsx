// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { Checkbox } from '@resources/components/ui/Checkbox';
import { DialogBody, DialogFooter } from '@resources/components/ui/Dialog';
import { Label } from '@resources/components/ui/Label';
import { Textarea } from '@resources/components/ui/Textarea';
import {
  useSaveHostServiceOverride,
  type HostServiceOverride,
  type HostServiceOverrideResult,
} from '@resources/hooks/useHostServices';
import type { HostTarget } from '@resources/hooks/useHostUpdates';
import { hostPermission } from '@resources/hooks/useHostUpdates';

type OverrideEditorProps = {
  host: HostTarget;
  unit: string;
  override: HostServiceOverride;
  onClose: () => void;
  onRevert: () => void;
};

export function OverrideEditor({ host, unit, override, onClose, onRevert }: OverrideEditorProps) {
  const { t } = useTranslation('environments');
  const save = useSaveHostServiceOverride(host);
  const [content, setContent] = useState(override.content || '[Service]\n');
  const [reload, setReload] = useState(true);
  const [verify, setVerify] = useState(true);
  const [result, setResult] = useState<HostServiceOverrideResult | null>(null);

  const handleSave = () => {
    save.mutate(
      { unit, content, reload, verify },
      {
        onSuccess: (data) => {
          setResult(data);
          if (!data.verifyOutput) onClose();
        },
      },
    );
  };

  return (
    <>
      <DialogBody className="flex flex-col gap-4">
        <section className="flex min-w-0 flex-col gap-1.5">
          <h4 className="text-xs font-medium text-muted-foreground">{t('hostServices.override.effective')}</h4>
          <pre
            className="max-h-64 overflow-auto rounded-lg border border-border bg-muted/40 p-3 font-mono text-xs text-foreground"
            aria-label={t('hostServices.override.effective')}
          >
            {override.effective}
          </pre>
        </section>
        <section className="flex min-w-0 flex-col gap-1.5">
          <Label htmlFor="host-service-override" className="text-xs font-medium text-muted-foreground">
            {t('hostServices.override.editor', { path: override.path })}
          </Label>
          <Textarea
            id="host-service-override"
            value={content}
            onChange={(evt) => setContent(evt.target.value)}
            spellCheck={false}
            className="min-h-[220px] font-mono text-xs"
          />
          {!override.exists && (
            <p className="text-xs text-muted-foreground">{t('hostServices.override.notYetCreated')}</p>
          )}
        </section>
        <div className="flex flex-wrap gap-6">
          <div className="flex items-center gap-2">
            <Checkbox
              id="host-service-reload"
              checked={reload}
              onCheckedChange={(value) => setReload(value === true)}
            />
            <Label htmlFor="host-service-reload" className="text-sm">
              {t('hostServices.override.reload')}
            </Label>
          </div>
          <div className="flex items-center gap-2">
            <Checkbox
              id="host-service-verify"
              checked={verify}
              onCheckedChange={(value) => setVerify(value === true)}
            />
            <Label htmlFor="host-service-verify" className="text-sm">
              {t('hostServices.override.verify')}
            </Label>
          </div>
        </div>
        {result?.verifyOutput && (
          <Alert variant="warning" title={t('hostServices.override.verifyWarning')}>
            <pre className="mt-1 font-mono text-xs break-words whitespace-pre-wrap">{result.verifyOutput}</pre>
          </Alert>
        )}
        {save.isError && (
          <p
            role="alert"
            className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-xs break-words whitespace-pre-wrap text-destructive"
          >
            {save.error.message}
          </p>
        )}
      </DialogBody>
      <DialogFooter className="justify-between">
        <Button
          variant="destructive"
          permission={hostPermission(host, 'servicesManage')}
          disabled={!override.exists || save.isPending}
          onClick={onRevert}
        >
          {t('hostServices.action.revert')}
        </Button>
        <div className="flex gap-2">
          <Button variant="ghost" onClick={onClose}>
            {result ? t('actions.close', { ns: 'common' }) : t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button permission={hostPermission(host, 'servicesManage')} disabled={save.isPending} onClick={handleSave}>
            {save.isPending ? t('actions.saving', { ns: 'common' }) : t('actions.save', { ns: 'common' })}
          </Button>
        </div>
      </DialogFooter>
    </>
  );
}
