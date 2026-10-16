// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconShieldPlus } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { toNumberOrNull, toNumberText } from '@resources/utils/numberText';
import { Select } from '@resources/components/ui/Select';
import { Alert } from '@resources/components/ui/Alert';
import { SettingsGroup } from '@resources/components/settings/SettingsGroup';
import { PERM } from '@core/auth/permissionKeys';
import { useAuth } from '@core/auth/useAuth';
import { usePermission } from '@core/auth/usePermission';
import {
  useCreateElevation,
  useDecideElevation,
  useElevatable,
  useMyElevations,
  usePendingElevations,
  useWithdrawElevation,
} from '@resources/hooks/useElevations';
import { ElevationRequestList } from './ElevationRequestList';
import { ElevationPolicyPanel } from './ElevationPolicyPanel';

/**
 * Security -> Elevations: request a permission for a bounded window, and
 * decide other people's requests.
 *
 * Both halves live on one tab because they are the same object seen from two
 * sides, and an approver is also a requester. The inbox only renders for
 * `users.manage`, which is what the decide route requires.
 *
 * The request form offers only permissions the catalog marks `elevatable`.
 * The backend rejects the rest with ErrPermissionNotElevatable; filtering here
 * means that error should never be the way a user discovers the rule.
 */
export function ElevationsTab() {
  const { t } = useTranslation('security');
  const { data: catalog } = useElevatable();
  const mine = useMyElevations();
  const pending = usePendingElevations();
  const myId = useAuth((s) => s.user?.id);
  const createElevation = useCreateElevation();
  const decide = useDecideElevation();
  const withdraw = useWithdrawElevation();
  const canApprove = usePermission(PERM.USERS_MANAGE);

  const [permissionKey, setPermissionKey] = useState('');
  const [justification, setJustification] = useState('');
  const [minutes, setMinutes] = useState('60');

  const elevatable = (catalog ?? []).filter((entry) => entry.elevatable);

  function submit() {
    const requestedMinutes = Number.parseInt(minutes, 10);
    if (!permissionKey || !justification.trim() || !Number.isFinite(requestedMinutes)) return;
    createElevation.mutate(
      { permissionKey, justification: justification.trim(), requestedMinutes },
      {
        onSuccess: () => {
          setPermissionKey('');
          setJustification('');
        },
      },
    );
  }

  return (
    <div className="space-y-6">
      <SettingsGroup
        icon={IconShieldPlus}
        title={t('elevations.requestTitle')}
        description={t('elevations.requestDescription')}
      >
        <div className="grid gap-3 sm:grid-cols-3">
          <label className="block space-y-1 sm:col-span-3">
            <span className="text-xs font-medium text-muted-foreground">{t('elevations.permission')}</span>
            <Select
              ariaLabel={t('elevations.permission')}
              value={permissionKey}
              onChange={setPermissionKey}
              placeholder={t('elevations.permissionPlaceholder')}
              searchable
              options={elevatable.map((entry) => ({
                value: entry.key,
                label: entry.description || entry.key,
                description: entry.key,
              }))}
            />
          </label>
          <label className="block space-y-1 sm:col-span-1">
            <span className="text-xs font-medium text-muted-foreground">{t('elevations.minutes')}</span>
            <NumberInput
              nullable
              min={1}
              max={1440}
              value={toNumberOrNull(minutes)}
              onChange={(next) => setMinutes(toNumberText(next))}
            />
          </label>
          <label className="block space-y-1 sm:col-span-2">
            <span className="text-xs font-medium text-muted-foreground">{t('elevations.justification')}</span>
            <Input
              value={justification}
              onChange={(e) => setJustification(e.target.value)}
              placeholder={t('elevations.justificationPlaceholder')}
            />
          </label>
        </div>

        {elevatable.length === 0 && <Alert variant="info" title={t('elevations.noneElevatable')} />}

        <div className="flex justify-end">
          <Button onClick={submit} disabled={createElevation.isPending || !permissionKey || !justification.trim()}>
            {t('elevations.submit')}
          </Button>
        </div>
      </SettingsGroup>

      <SettingsGroup
        icon={IconShieldPlus}
        title={t('elevations.mineTitle')}
        description={t('elevations.mineDescription')}
      >
        <ElevationRequestList
          requests={mine.data ?? []}
          loading={mine.isLoading}
          emptyLabel={t('elevations.noneMine')}
          onWithdraw={(id) => withdraw.mutate(id)}
        />
      </SettingsGroup>

      {/* Only an approver sees the inbox: users.manage is what the decide
          route requires, and an empty panel would be a worse answer than
          no panel. */}
      {canApprove && (
        <SettingsGroup
          icon={IconShieldPlus}
          title={t('elevations.inboxTitle')}
          description={t('elevations.inboxDescription')}
        >
          <ElevationRequestList
            requests={(pending.data ?? []).filter((r) => r.requesterId !== myId)}
            loading={pending.isLoading}
            emptyLabel={t('elevations.nonePending')}
            busy={decide.isPending}
            onDecide={(id, approve) => decide.mutate({ id, approve })}
          />
        </SettingsGroup>
      )}

      {canApprove && (
        <SettingsGroup
          icon={IconShieldPlus}
          title={t('elevations.policyTitle')}
          description={t('elevations.policyDescription')}
        >
          <ElevationPolicyPanel />
        </SettingsGroup>
      )}
    </div>
  );
}
