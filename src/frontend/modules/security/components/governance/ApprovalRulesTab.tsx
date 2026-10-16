// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconGavel, IconPlus } from '@tabler/icons-react';
import { SettingsGroup } from '@resources/components/settings/SettingsGroup';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { PERM } from '@core/auth/permissionKeys';
import { useEnvironmentList } from '@resources/hooks/useEnvironmentList';
import { useApprovalRules, useDeleteApprovalRule, type ApprovalRule } from '../../hooks/useApprovals';
import { ApprovalRuleDialog } from './ApprovalRuleDialog';

export function ApprovalRulesTab() {
  const { t } = useTranslation('security');
  const rules = useApprovalRules();
  const environments = useEnvironmentList();
  const remove = useDeleteApprovalRule();
  const [editing, setEditing] = useState<{ rule: ApprovalRule | null } | null>(null);
  const [deleting, setDeleting] = useState<ApprovalRule | null>(null);
  const envName = (id: string | null) =>
    id ? (environments.data?.find((e) => e.id === id)?.name ?? id) : t('governance.rules.allEnvironments');

  return (
    <SettingsGroup
      icon={IconGavel}
      title={t('governance.rules.title')}
      description={t('governance.rules.description')}
      actions={
        <Button size="sm" permission={PERM.APPROVALS_MANAGE} onClick={() => setEditing({ rule: null })}>
          <IconPlus className="size-4" />
          {t('governance.rules.add')}
        </Button>
      }
    >
      {rules.isLoading && (
        <TypographyText size="sm" tone="muted">
          {t('common:common.loading')}
        </TypographyText>
      )}
      {rules.data && rules.data.length === 0 && (
        <TypographyText size="sm" tone="muted">
          {t('governance.rules.empty')}
        </TypographyText>
      )}
      <ul className="space-y-2">
        {(rules.data ?? []).map((rule) => (
          <li
            key={rule.id}
            className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-border p-3"
          >
            <div className="min-w-0 space-y-1">
              <div className="flex flex-wrap items-center gap-2">
                <span className="text-sm font-semibold text-foreground">
                  {t(`governance.categories.${rule.category}`)}
                </span>
                <Badge variant={rule.enabled ? 'success' : 'secondary'}>
                  {rule.enabled ? t('governance.rules.on') : t('governance.rules.off')}
                </Badge>
              </div>
              <TypographyText size="xs" tone="muted">
                {t('governance.rules.summary', {
                  count: rule.requiredApprovers,
                  permission: rule.approverPermission,
                  minutes: rule.expiryMinutes,
                  environment: envName(rule.environmentId),
                })}
              </TypographyText>
              {rule.description && <TypographyText size="xs">{rule.description}</TypographyText>}
            </div>
            <div className="flex gap-2">
              <Button
                size="sm"
                variant="outline"
                permission={PERM.APPROVALS_MANAGE}
                onClick={() => setEditing({ rule })}
              >
                {t('common:actions.edit')}
              </Button>
              <Button size="sm" variant="ghost" permission={PERM.APPROVALS_MANAGE} onClick={() => setDeleting(rule)}>
                {t('common:actions.delete')}
              </Button>
            </div>
          </li>
        ))}
      </ul>

      {editing && (
        <ApprovalRuleDialog
          key={editing.rule?.id ?? 'new'}
          open
          rule={editing.rule}
          onOpenChange={(openState) => !openState && setEditing(null)}
        />
      )}
      <MutationConfirmDialog
        open={deleting !== null}
        onOpenChange={(isOpen) => !isOpen && setDeleting(null)}
        title={t('governance.rules.deleteTitle')}
        description={t('governance.rules.deleteDescription', {
          category: deleting ? t(`governance.categories.${deleting.category}`) : '',
        })}
        permission={PERM.APPROVALS_MANAGE}
        mutation={remove}
        variables={deleting?.id ?? ''}
      />
    </SettingsGroup>
  );
}
