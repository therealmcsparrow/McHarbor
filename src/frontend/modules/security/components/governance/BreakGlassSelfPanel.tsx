// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlarm } from '@tabler/icons-react';
import { SettingsGroup } from '@resources/components/settings/SettingsGroup';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { toNumberOrNull, toNumberText } from '@resources/utils/numberText';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { Textarea } from '@resources/components/ui/Textarea';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { formatDate } from '@resources/utils/format';
import { useActivateBreakGlass, useBreakGlassStatus, useEndBreakGlass } from '../../hooks/useBreakGlass';

export function BreakGlassSelfPanel() {
  const { t } = useTranslation('security');
  const statusValue = useBreakGlassStatus();
  const activate = useActivateBreakGlass();
  const end = useEndBreakGlass();
  const [activating, setActivating] = useState(false);
  const [ending, setEnding] = useState(false);
  const [reason, setReason] = useState('');
  const [minutes, setMinutes] = useState('30');
  const [password, setPassword] = useState('');

  const st = statusValue.data;
  if (!st) return null;
  const max = st.maxMinutes;
  const wanted = Number.parseInt(minutes, 10);
  const valid =
    reason.trim().length >= 10 && Number.isFinite(wanted) && wanted >= 5 && wanted <= max && password !== '';

  return (
    <SettingsGroup
      icon={IconAlarm}
      title={t('governance.breakGlass.selfTitle')}
      description={t('governance.breakGlass.selfDescription')}
    >
      {!st.designated && (
        <TypographyText size="sm" tone="muted">
          {t('governance.breakGlass.notDesignated')}
        </TypographyText>
      )}
      {st.active && (
        <Alert
          variant="destructive"
          title={t('governance.breakGlass.activeTitle', { date: formatDate(st.active.expiresAt) })}
          description={st.active.reason}
          action={
            <Button size="sm" variant="outline" onClick={() => setEnding(true)}>
              {t('governance.breakGlass.end')}
            </Button>
          }
        />
      )}
      {st.designated && !st.active && (
        <div className="flex justify-end">
          <Button variant="destructive" onClick={() => setActivating(true)}>
            {t('governance.breakGlass.activate')}
          </Button>
        </div>
      )}
      <MutationConfirmDialog
        open={activating}
        onOpenChange={(isOpen) => {
          setActivating(isOpen);
          if (!isOpen) {
            setReason('');
            setPassword('');
          }
        }}
        title={t('governance.breakGlass.activateTitle')}
        description={t('governance.breakGlass.activateDescription', { max })}
        confirmLabel={t('governance.breakGlass.activate')}
        mutation={activate}
        variables={{ reason: reason.trim(), minutes: wanted, password }}
        disabled={!valid}
      >
        <label className="block space-y-1">
          <span className="text-xs font-medium text-muted-foreground">{t('governance.breakGlass.reason')}</span>
          <Textarea
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            rows={3}
            placeholder={t('governance.breakGlass.reasonPlaceholder')}
          />
        </label>
        <label className="block space-y-1">
          <span className="text-xs font-medium text-muted-foreground">
            {t('governance.breakGlass.minutes', { max })}
          </span>
          <NumberInput
            nullable
            allowOutOfRange
            min={5}
            max={max}
            value={toNumberOrNull(minutes)}
            onChange={(next) => setMinutes(toNumberText(next))}
          />
        </label>
        <label className="block space-y-1">
          <span className="text-xs font-medium text-muted-foreground">{t('governance.breakGlass.password')}</span>
          <Input
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
          <TypographyText size="xs" tone="muted">
            {t('governance.breakGlass.passwordHint')}
          </TypographyText>
        </label>
      </MutationConfirmDialog>
      <MutationConfirmDialog
        open={ending}
        onOpenChange={setEnding}
        title={t('governance.breakGlass.endTitle')}
        description={t('governance.breakGlass.endDescription')}
        confirmLabel={t('governance.breakGlass.end')}
        variant="caution"
        mutation={end}
        variables={null}
      />
    </SettingsGroup>
  );
}
