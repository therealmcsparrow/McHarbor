// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { PERM } from '@core/auth/permissionKeys';
import { usePermissionCatalog } from '@core/auth/usePermission';
import { useEnvironmentList } from '@resources/hooks/useEnvironmentList';
import {
  useApprovalCategories,
  useSaveApprovalRule,
  type ApprovalRule,
  type ApprovalRuleInput,
} from '../../hooks/useApprovals';

type ApprovalRuleDialogProps = {
  open: boolean;
  rule: ApprovalRule | null;
  onOpenChange: (open: boolean) => void;
};

const EMPTY: ApprovalRuleInput = {
  category: '',
  environmentId: null,
  enabled: true,
  requiredApprovers: 1,
  approverPermission: PERM.APPROVALS_DECIDE,
  expiryMinutes: 1440,
  description: '',
};

function toInput(rule: ApprovalRule | null): ApprovalRuleInput {
  if (!rule) return EMPTY;
  const { category, environmentId, enabled, requiredApprovers, approverPermission, expiryMinutes, description } = rule;
  return { category, environmentId, enabled, requiredApprovers, approverPermission, expiryMinutes, description };
}

export function ApprovalRuleDialog({ open: isOpen, rule, onOpenChange }: ApprovalRuleDialogProps) {
  const { t } = useTranslation('security');
  const categories = useApprovalCategories(isOpen);
  const catalog = usePermissionCatalog();
  const environments = useEnvironmentList();
  const save = useSaveApprovalRule();
  const [draft, setDraft] = useState<ApprovalRuleInput>(() => toInput(rule));
  const set = (patch: Partial<ApprovalRuleInput>) => setDraft((d) => ({ ...d, ...patch }));
  const selected = categories.data?.find((c) => c.key === draft.category);

  const submit = () => save.mutate({ id: rule?.id, input: draft }, { onSuccess: () => onOpenChange(false) });

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{rule ? t('governance.rules.editTitle') : t('governance.rules.createTitle')}</DialogTitle>
          <DialogDescription>{t('governance.rules.dialogDescription')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <Select
            ariaLabel={t('governance.rules.category')}
            placeholder={t('governance.rules.category')}
            value={draft.category}
            onChange={(category) => set({ category })}
            options={(categories.data ?? []).map((c) => ({ value: c.key, label: t(`governance.categories.${c.key}`) }))}
          />
          {selected && (
            <div className="rounded-md bg-muted p-2">
              <TypographyText size="xs" tone="muted">
                {t('governance.rules.matches')}
              </TypographyText>
              {selected.routes.map((r) => (
                <TypographyText key={`${r.method} ${r.pattern}`} size="xs" className="font-mono">
                  {r.method} {r.pattern}
                </TypographyText>
              ))}
            </div>
          )}
          <Select
            ariaLabel={t('governance.rules.environment')}
            value={draft.environmentId ?? ''}
            onChange={(v) => set({ environmentId: v || null })}
            options={[
              { value: '', label: t('governance.rules.allEnvironments') },
              ...(environments.data ?? []).map((e) => ({ value: e.id, label: e.name })),
            ]}
          />
          <div className="grid gap-3 sm:grid-cols-2">
            <label className="block space-y-1">
              <span className="text-xs font-medium text-muted-foreground">
                {t('governance.rules.requiredApprovers')}
              </span>
              <NumberInput
                min={1}
                max={5}
                value={draft.requiredApprovers}
                onChange={(requiredApprovers) => set({ requiredApprovers })}
              />
            </label>
            <label className="block space-y-1">
              <span className="text-xs font-medium text-muted-foreground">{t('governance.rules.expiryMinutes')}</span>
              <NumberInput
                min={5}
                max={10080}
                value={draft.expiryMinutes}
                onChange={(expiryMinutes) => set({ expiryMinutes })}
              />
            </label>
          </div>
          <Select
            ariaLabel={t('governance.rules.approverPermission')}
            value={draft.approverPermission}
            onChange={(approverPermission) => set({ approverPermission })}
            options={(catalog.data?.map((e) => e.key) ?? [PERM.APPROVALS_DECIDE]).map((key) => ({
              value: key,
              label: key,
            }))}
          />
          <Input
            aria-label={t('governance.rules.descriptionLabel')}
            placeholder={t('governance.rules.descriptionLabel')}
            value={draft.description}
            onChange={(e) => set({ description: e.target.value })}
          />
          <label className="flex items-center gap-2 text-sm">
            <Switch
              checked={draft.enabled}
              onCheckedChange={(enabled) => set({ enabled })}
              aria-label={t('governance.rules.enabled')}
            />
            {t('governance.rules.enabled')}
          </label>
          {save.isError && (
            <p role="alert" className="text-xs text-destructive">
              {save.error.message}
            </p>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('common:actions.cancel')}
          </Button>
          <Button onClick={submit} disabled={!draft.category || save.isPending} permission={PERM.APPROVALS_MANAGE}>
            {t('common:actions.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
