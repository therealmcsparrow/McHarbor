// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconLifebuoy } from '@tabler/icons-react';
import { SettingsGroup } from '@resources/components/settings/SettingsGroup';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { toNumberOrNull, toNumberText } from '@resources/utils/numberText';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { Select } from '@resources/components/ui/Select';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { formatDate } from '@resources/utils/format';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { useUsers } from '@resources/hooks/useUsers';
import {
  useBreakGlassAccounts,
  useBreakGlassSessions,
  useDesignateBreakGlass,
  useEndBreakGlass,
  useUndesignateBreakGlass,
  type BreakGlassAccount,
} from '../../hooks/useBreakGlass';

export function BreakGlassAdminPanel() {
  const { t } = useTranslation('security');
  const canView = usePermission(PERM.USERS_VIEW);
  const accounts = useBreakGlassAccounts(canView);
  const sessions = useBreakGlassSessions(canView);
  const users = useUsers();
  const designate = useDesignateBreakGlass();
  const undesignate = useUndesignateBreakGlass();
  const end = useEndBreakGlass();
  const [userId, setUserId] = useState('');
  const [maxMinutes, setMaxMinutes] = useState('60');
  const [removing, setRemoving] = useState<BreakGlassAccount | null>(null);
  const [ending, setEnding] = useState<BreakGlassAccount | null>(null);
  if (!canView) return null;

  const submit = () =>
    designate.mutate(
      { userId, maxMinutes: Number.parseInt(maxMinutes, 10) || 60, note: '' },
      { onSuccess: () => setUserId('') },
    );

  return (
    <SettingsGroup
      icon={IconLifebuoy}
      title={t('governance.breakGlass.adminTitle')}
      description={t('governance.breakGlass.adminDescription')}
    >
      <div className="flex flex-wrap items-end gap-2">
        <div className="w-64">
          <Select
            ariaLabel={t('governance.breakGlass.user')}
            placeholder={t('governance.breakGlass.user')}
            value={userId}
            onChange={setUserId}
            options={(users.data ?? []).map((u) => ({ value: u.id, label: u.username }))}
          />
        </div>
        <label className="block w-40 space-y-1">
          <span className="text-xs font-medium text-muted-foreground">{t('governance.breakGlass.maxMinutes')}</span>
          <NumberInput
            nullable
            min={5}
            max={240}
            value={toNumberOrNull(maxMinutes)}
            onChange={(next) => setMaxMinutes(toNumberText(next))}
          />
        </label>
        <Button onClick={submit} disabled={!userId || designate.isPending} permission={PERM.ACCOUNT_MANAGE}>
          {t('governance.breakGlass.designate')}
        </Button>
      </div>
      {designate.isError && (
        <p role="alert" className="text-xs text-destructive">
          {designate.error.message}
        </p>
      )}

      <ul className="space-y-2">
        {(accounts.data ?? []).map((a) => (
          <li
            key={a.id}
            className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-border p-3"
          >
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-sm font-medium text-foreground">{a.username}</span>
              <TypographyText size="xs" tone="muted">
                {t('governance.breakGlass.upTo', { count: a.maxMinutes })}
              </TypographyText>
              {a.active && (
                <Badge variant="destructive">
                  {t('governance.breakGlass.activeUntil', { date: formatDate(a.active.expiresAt) })}
                </Badge>
              )}
            </div>
            <div className="flex gap-2">
              {a.active && (
                <Button size="sm" variant="outline" permission={PERM.ACCOUNT_MANAGE} onClick={() => setEnding(a)}>
                  {t('governance.breakGlass.end')}
                </Button>
              )}
              <Button size="sm" variant="ghost" permission={PERM.ACCOUNT_MANAGE} onClick={() => setRemoving(a)}>
                {t('governance.breakGlass.undesignate')}
              </Button>
            </div>
          </li>
        ))}
      </ul>

      <div className="space-y-1">
        <TypographyText size="xs" weight="semibold">
          {t('governance.breakGlass.history')}
        </TypographyText>
        {(sessions.data ?? []).length === 0 && (
          <TypographyText size="xs" tone="muted">
            {t('governance.breakGlass.noHistory')}
          </TypographyText>
        )}
        <ul className="space-y-1">
          {(sessions.data ?? []).map((s) => (
            <li key={s.id} className="text-xs text-muted-foreground">
              {t('governance.breakGlass.historyRow', {
                user: s.username,
                start: formatDate(s.startedAt),
                reason: s.reason,
              })}
            </li>
          ))}
        </ul>
      </div>

      <MutationConfirmDialog
        open={removing !== null}
        onOpenChange={(openState) => !openState && setRemoving(null)}
        title={t('governance.breakGlass.undesignateTitle')}
        description={t('governance.breakGlass.undesignateDescription', { user: removing?.username ?? '' })}
        permission={PERM.ACCOUNT_MANAGE}
        mutation={undesignate}
        variables={removing?.userId ?? ''}
      />
      <MutationConfirmDialog
        open={ending !== null}
        onOpenChange={(isOpen) => !isOpen && setEnding(null)}
        title={t('governance.breakGlass.endTitle')}
        description={t('governance.breakGlass.endOtherDescription', { user: ending?.username ?? '' })}
        confirmLabel={t('governance.breakGlass.end')}
        permission={PERM.ACCOUNT_MANAGE}
        mutation={end}
        variables={ending?.userId ?? null}
      />
    </SettingsGroup>
  );
}
