// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconWand } from '@tabler/icons-react';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Textarea } from '@resources/components/ui/Textarea';
import { Label } from '@resources/components/ui/Label';
import { PERM } from '@core/auth/permissionKeys';
import { PermissionEditor, type PermissionSelection } from '@resources/components/security/PermissionEditor';
import type { Policy, PolicyInput } from '@resources/hooks/usePolicies';
import { usePolicyReference } from '@resources/hooks/usePolicyReference';
import { ExpressionReference } from '@resources/components/security/abac/ExpressionReference';
import { PolicyBasicsFields } from '@resources/components/security/abac/PolicyBasicsFields';

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  initial: Policy | null;
  onSubmit: (input: PolicyInput) => void;
  isSubmitting: boolean;
};

const BLANK: PolicyInput = {
  name: '',
  description: '',
  effect: 'allow',
  resourceType: 'container',
  expression: 'hasLabel(resource.labels, "env") == "prod"',
  priority: 100,
  enabled: false,
  permissions: [],
};

const toSelection = (keys: string[]): PermissionSelection => ({
  wildcard: false,
  overrides: Object.fromEntries(keys.map((k) => [k, 'grant' as const])),
});

export function PolicyEditorDialog({ open: isOpen, onOpenChange, initial, onSubmit, isSubmitting }: Props) {
  const { t } = useTranslation('abac');
  const reference = usePolicyReference(isOpen);
  const [form, setForm] = useState<PolicyInput>(BLANK);
  const [selection, setSelection] = useState<PermissionSelection>(toSelection([]));
  const exprRef = useRef<HTMLTextAreaElement>(null);

  const [synced, setSynced] = useState<{ initial: typeof initial; isOpen: boolean } | null>(null);
  if (!synced || synced.initial !== initial || synced.isOpen !== isOpen) {
    setSynced({ initial, isOpen });
    const next = initial
      ? {
          name: initial.name,
          description: initial.description,
          effect: initial.effect,
          resourceType: initial.resourceType,
          expression: initial.expression,
          priority: initial.priority,
          enabled: initial.enabled,
          permissions: initial.permissions ?? [],
        }
      : BLANK;
    setForm(next);
    setSelection(toSelection(next.permissions));
  }

  const resourceType = useMemo(
    () => reference.data?.resourceTypes.find((rt) => rt.type === form.resourceType),
    [reference.data, form.resourceType],
  );
  const permissions = Object.keys(selection.overrides).filter((k) => selection.overrides[k] === 'grant');

  const insert = (text: string) => {
    const el = exprRef.current;
    const start = el?.selectionStart ?? form.expression.length;
    const end = el?.selectionEnd ?? form.expression.length;
    const expression = form.expression.slice(0, start) + text + form.expression.slice(end);
    setForm({ ...form, expression });
    requestAnimationFrame(() => {
      el?.focus();
      el?.setSelectionRange(start + text.length, start + text.length);
    });
  };

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-4xl">
        <DialogHeader>
          <DialogTitle>{initial ? t('editor.titleEdit') : t('editor.titleCreate')}</DialogTitle>
        </DialogHeader>
        <DialogBody className="space-y-4">
          <PolicyBasicsFields form={form} types={reference.data?.resourceTypes ?? []} onChange={setForm} />
          <div className="space-y-1">
            <Label>{t('editor.permissions')}</Label>
            <PermissionEditor
              value={selection}
              onChange={setSelection}
              selectOnly
              selectLabels={{
                include: t('editor.include'),
                includeAll: t('editor.includeAll'),
                clear: t('editor.clear'),
              }}
              listClassName="max-h-[240px]"
            />
            <p className="text-xs text-muted-foreground">{t('editor.permissionsHintSelect')}</p>
          </div>
          <div className="space-y-1">
            <Label htmlFor="abac-expr" className="flex items-center gap-2">
              <IconWand className="size-3.5" aria-hidden="true" />
              {t('editor.expression')}
            </Label>
            <p className="text-xs text-muted-foreground">{t('reference.intro')}</p>
            <div className="grid grid-cols-1 gap-3 lg:grid-cols-[1fr_1fr]">
              <Textarea
                id="abac-expr"
                ref={exprRef}
                value={form.expression}
                onChange={(e) => setForm({ ...form, expression: e.target.value })}
                className="min-h-[224px] font-mono text-xs"
              />
              <ExpressionReference
                reference={reference.data}
                resourceType={resourceType}
                onInsert={insert}
                onReplace={(expression) => setForm({ ...form, expression })}
              />
            </div>
          </div>
        </DialogBody>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            {t('editor.cancel')}
          </Button>
          <Button
            permission={PERM.RBAC_POLICIES_MANAGE}
            onClick={() => onSubmit({ ...form, permissions })}
            disabled={!form.name || !form.expression || isSubmitting}
          >
            {isSubmitting ? t('editor.saving') : t('editor.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
