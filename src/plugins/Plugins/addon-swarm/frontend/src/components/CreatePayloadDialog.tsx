// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconUpload } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Input } from '@resources/components/ui/Input';
import { Textarea } from '@resources/components/ui/Textarea';
import { Spinner } from '@resources/components/ui/Spinner';

type CreatePayloadDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: string;
  /** Permission key for the submit button. */
  permission: string;
  pending: boolean;
  /** Receives the name and the payload already base64-encoded. */
  onSubmit: (name: string, base64: string) => void;
};

/**
 * Create a secret or a config.
 *
 * One dialog for both because the write side is identical — a name and some
 * bytes. They diverge only on the read side, where a secret has nothing to
 * show and a config does.
 *
 * The value can be typed or uploaded. Both paths end at base64, which is what
 * lets an arbitrary binary payload survive a JSON body unharmed.
 */
export function CreatePayloadDialog({
  open: isOpen,
  onOpenChange,
  title,
  description,
  permission,
  pending,
  onSubmit,
}: CreatePayloadDialogProps) {
  const { t } = useTranslation('swarm');
  const [itemName, setName] = useState('');
  const [value, setValue] = useState('');
  const [fileName, setFileName] = useState('');
  const [fileBase64, setFileBase64] = useState('');
  const fileInput = useRef<HTMLInputElement>(null);

  const reset = () => {
    setName('');
    setValue('');
    setFileName('');
    setFileBase64('');
  };

  const closeDialog = (next: boolean) => {
    if (!next) reset();
    onOpenChange(next);
  };

  const handleFile = (file: File | undefined) => {
    if (!file) return;
    const reader = new FileReader();
    reader.onload = () => {
      // readAsDataURL gives `data:<mime>;base64,<payload>`; the payload is
      // everything after the comma.
      const result = typeof reader.result === 'string' ? reader.result : '';
      setFileBase64(result.slice(result.indexOf(',') + 1));
      setFileName(file.name);
    };
    reader.readAsDataURL(file);
  };

  const encoded = fileBase64 || (value ? toBase64(value) : '');
  const canSubmit = itemName.trim() !== '' && encoded !== '';

  return (
    <Dialog open={isOpen} onOpenChange={closeDialog}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          <label className="block space-y-1.5">
            <span className="text-sm font-medium text-foreground">{t('fields.name')}</span>
            <Input value={itemName} onChange={(e) => setName(e.target.value)} />
          </label>

          <label className="block space-y-1.5">
            <span className="text-sm font-medium text-foreground">{t('fields.value')}</span>
            <Textarea
              value={value}
              onChange={(e) => {
                setValue(e.target.value);
                // Typing supersedes a previously chosen file, so the two
                // sources can never disagree about what is being submitted.
                setFileBase64('');
                setFileName('');
              }}
              rows={6}
              className="font-mono text-xs"
              disabled={fileBase64 !== ''}
            />
          </label>

          <div className="flex items-center gap-3">
            <Button variant="outline" size="sm" onClick={() => fileInput.current?.click()}>
              <IconUpload className="size-4" />
              {t('actions.uploadFile')}
            </Button>
            {fileName && <span className="text-xs text-muted-foreground">{fileName}</span>}
            <input
              ref={fileInput}
              type="file"
              className="hidden"
              aria-label={t('actions.uploadFile')}
              onChange={(e) => handleFile(e.target.files?.[0])}
            />
          </div>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => closeDialog(false)}>
            {t('actions.cancel')}
          </Button>
          <Button
            permission={permission}
            disabled={!canSubmit || pending}
            onClick={() => onSubmit(itemName.trim(), encoded)}
          >
            {pending && <Spinner size="sm" />}
            {t('actions.create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * UTF-8 safe base64.
 *
 * `btoa` throws on any code point above U+00FF, so a config containing a
 * single accented character would fail with an unhelpful DOM exception.
 */
function toBase64(input: string): string {
  const bytes = new TextEncoder().encode(input);
  let binary = '';
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary);
}
