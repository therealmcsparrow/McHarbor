// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlayerPlay } from '@tabler/icons-react';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Textarea } from '@resources/components/ui/Textarea';
import { CodeEditor } from '@resources/components/CodeEditor';
import { Alert } from '@resources/components/ui/Alert';
import { PERM } from '@core/auth/permissionKeys';
import type { CustomNodeWithCode } from '../hooks/useCustomNodeSync';
import { CUSTOM_NODE_CATEGORIES, useCustomNodeForm } from '../hooks/useCustomNodeForm';

type CustomNodeDialogProps = {
  open: boolean;
  node?: CustomNodeWithCode;
  onClose: () => void;
};

/** One labelled field. Keeps the form grid readable. */
function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block space-y-1">
      <span className="text-xs font-medium text-muted-foreground">{label}</span>
      {children}
    </label>
  );
}

/**
 * Create or edit one custom workflow node: its identity, its ports, its script,
 * and a dry run of that script against a message the author supplies.
 *
 * State and mutations live in `useCustomNodeForm`; this file is the rendering
 * half only.
 */
export function CustomNodeDialog({ open: isOpen, node, onClose }: CustomNodeDialogProps) {
  const { t } = useTranslation('common');
  const form = useCustomNodeForm(node, onClose);
  const f = form.fields;

  return (
    <Dialog open={isOpen} onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>{form.editing ? t('customNodes.editNode') : t('customNodes.createNode')}</DialogTitle>
        </DialogHeader>
        <DialogBody className="space-y-4">
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label={t('customNodes.nodeKey')}>
              {/* The key is the node's identity on disk and inside saved
                  workflows, so it is fixed once created. */}
              <Input value={f.key} onChange={(e) => f.setKey(e.target.value)} disabled={form.editing} />
            </Field>
            <Field label={t('customNodes.nodeLabel')}>
              <Input value={f.label} onChange={(e) => f.setLabel(e.target.value)} />
            </Field>
            <Field label={t('customNodes.nodeCategory')}>
              <select
                aria-label={t('customNodes.nodeCategory')}
                value={f.category}
                onChange={(e) => f.setCategory(e.target.value as CustomNodeWithCode['category'])}
                className="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground"
              >
                {CUSTOM_NODE_CATEGORIES.map((c) => (
                  <option key={c} value={c}>
                    {c}
                  </option>
                ))}
              </select>
            </Field>
            <Field label={t('customNodes.nodeIcon')}>
              <Input value={f.icon} onChange={(e) => f.setIcon(e.target.value)} />
            </Field>
            <Field label={t('customNodes.inputPorts')}>
              <Input value={f.inputPorts} onChange={(e) => f.setInputPorts(e.target.value)} />
            </Field>
            <Field label={t('customNodes.outputPorts')}>
              <Input value={f.outputPorts} onChange={(e) => f.setOutputPorts(e.target.value)} />
            </Field>
          </div>

          <Field label={t('customNodes.nodeDescription')}>
            <Input value={f.description} onChange={(e) => f.setDescription(e.target.value)} />
          </Field>

          <Field label={t('customNodes.script')}>
            {/* The repo already ships a CodeMirror wrapper with a JavaScript
                mode; a plain textarea for a scripting surface would be a
                downgrade from what is available. */}
            <CodeEditor value={f.code} onChange={f.setCode} language="javascript" minHeight="220px" />
            <span className="text-xs text-muted-foreground">{t('customNodes.scriptHelp')}</span>
          </Field>

          <Field label={t('customNodes.testInput')}>
            <Textarea
              value={f.testMsg}
              onChange={(e) => f.setTestMsg(e.target.value)}
              className="min-h-[80px] font-mono text-xs"
              spellCheck={false}
            />
          </Field>

          {form.testMsgError && <Alert variant="warning" title={form.testMsgError} />}

          {form.testResult && (
            <Alert
              variant={form.testResult.error ? 'destructive' : 'success'}
              title={t('customNodes.testResult')}
              description={
                form.testResult.error
                  ? form.testResult.error
                  : [
                      `${form.testResult.port} - ${form.testResult.duration}ms`,
                      // Go marshals a nil []string as JSON null, so a run
                      // that logged nothing arrives as `logs: null`, not `[]`,
                      // and spreading it throws.
                      ...(form.testResult.logs ?? []),
                    ].join('\n')
              }
            />
          )}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={form.runTest} disabled={form.testing || !f.code.trim()}>
            <IconPlayerPlay className="size-4" aria-hidden="true" />
            {t('customNodes.testScript')}
          </Button>
          <Button
            permission={PERM.CUSTOM_NODES_MANAGE}
            onClick={form.save}
            disabled={form.saving || !f.key.trim() || !f.label.trim()}
          >
            {t('actions.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
