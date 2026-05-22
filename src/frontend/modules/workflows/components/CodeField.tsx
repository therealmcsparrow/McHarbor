// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useDeferredValue } from 'react';
import { useTranslation } from 'react-i18next';
import { IconMaximize } from '@tabler/icons-react';
import { CodeEditor } from '@resources/components/CodeEditor';
import { Button } from '@resources/components/ui/Button';
import { Label } from '@resources/components/ui/Label';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@resources/components/ui/Dialog';
import type { ConfigField } from '../types';

interface CodeFieldProps {
  field: ConfigField;
  value: string;
  onChange: (v: string) => void;
  language: 'javascript' | 'typescript';
  fieldLabel: string;
}

export function CodeField({ field, value, onChange, language, fieldLabel }: CodeFieldProps) {
  const { t } = useTranslation('common');
  const [modalOpen, setModalOpen] = useState(false);
  const deferredValue = useDeferredValue(value);

  return (
    <div>
      <div className="mb-1.5 flex items-center justify-between">
        <Label className="text-xs">
          {fieldLabel}
          {field.required && <span className="text-destructive"> *</span>}
        </Label>
        <Button
          variant="ghost"
          size="icon"
          aria-label={t('expand')}
          className="size-6"
          onClick={() => setModalOpen(true)}
        >
          <IconMaximize className="size-3.5" />
        </Button>
      </div>

      <CodeEditor
        value={deferredValue}
        onChange={onChange}
        language={language}
        minHeight="120px"
        className="max-h-[200px] overflow-auto"
      />

      <Dialog open={modalOpen} onOpenChange={setModalOpen}>
        <DialogContent className="max-w-4xl h-[75vh] p-0">
          <DialogHeader>
            <DialogTitle>{fieldLabel}</DialogTitle>
            <DialogDescription className="sr-only">
              {fieldLabel}
            </DialogDescription>
          </DialogHeader>
          <DialogBody className="p-0">
            <div className="min-h-0 px-4 py-2">
              <CodeEditor
                value={value}
                onChange={onChange}
                language={language}
                minHeight="100%"
                className="h-full"
              />
            </div>
          </DialogBody>
        </DialogContent>
      </Dialog>
    </div>
  );
}
