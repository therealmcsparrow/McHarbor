// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { IconLogout2 } from '@tabler/icons-react';
import { PERM } from '@core/auth/permissionKeys';
import { Button } from '@resources/components/ui/Button';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { toNumberOrNull, toNumberText } from '@resources/utils/numberText';
import { Label } from '@resources/components/ui/Label';
import { Switch } from '@resources/components/ui/Switch';
import { Flex } from '@resources/components/ui/Layout';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { Spinner } from '@resources/components/ui/Spinner';
import { toast } from 'sonner';
import {
  useIdleTimeout,
  useSaveIdleTimeout,
  DEFAULT_IDLE_SECONDS,
  MIN_IDLE_SECONDS,
  MAX_IDLE_SECONDS,
} from '../hooks/useIdleTimeout';

/**
 * Automatic sign-out. Stored as a single number of seconds where 0 means
 * "never" — the switch is a friendlier face on that one value rather
 * than a second setting that could disagree with it.
 *
 * The number is deliberately in seconds, not minutes: it is what the
 * server stores, what the API validates, and what an operator writing
 * this into a compliance document has to quote.
 */
export function AutoLogoutCard() {
  const { t } = useTranslation('security');
  const { data: currentSeconds, isLoading } = useIdleTimeout();
  const save = useSaveIdleTimeout();

  const [enabled, setEnabled] = useState(true);
  const [seconds, setSeconds] = useState<string>('');
  const [error, setError] = useState('');

  const [syncedSeconds, setSyncedSeconds] = useState<typeof currentSeconds>(undefined);
  if (currentSeconds !== syncedSeconds) {
    setSyncedSeconds(currentSeconds);
    if (currentSeconds !== undefined) {
      const stored = currentSeconds ?? DEFAULT_IDLE_SECONDS;
      setEnabled(stored > 0);
      setSeconds(String(stored > 0 ? stored : DEFAULT_IDLE_SECONDS));
    }
  }

  function handleSubmit(evt: FormEvent<HTMLFormElement>) {
    evt.preventDefault();

    let value = 0;
    if (enabled) {
      const parsed = parseInt(seconds, 10);
      if (!Number.isFinite(parsed) || parsed < MIN_IDLE_SECONDS || parsed > MAX_IDLE_SECONDS) {
        setError(
          t('password.autoLogout.invalid', {
            min: MIN_IDLE_SECONDS,
            max: MAX_IDLE_SECONDS,
          }),
        );
        return;
      }
      value = parsed;
    }

    setError('');
    save.mutate(value, {
      onSuccess: () => toast.success(t('toast.settingsSaved')),
    });
  }

  if (isLoading) {
    return (
      <Flex align="center" justify="center" className="py-8">
        <Spinner size="lg" />
      </Flex>
    );
  }

  const parsedSeconds = parseInt(seconds, 10);
  const showsEquivalent = enabled && Number.isFinite(parsedSeconds) && parsedSeconds >= 60;

  return (
    <div className="space-y-6">
      <div>
        <Flex align="center" gap={2}>
          <IconLogout2 className="size-4 text-muted-foreground" />
          <h2 className="text-base font-semibold text-foreground">{t('password.autoLogout.title')}</h2>
        </Flex>
        <p className="mt-1 text-sm text-muted-foreground">{t('password.autoLogout.description')}</p>
      </div>

      <form className="max-w-md space-y-4" onSubmit={handleSubmit}>
        <Flex align="center" justify="between" gap={4}>
          <div className="min-w-0">
            <Label htmlFor="auto-logout-enabled" className="text-xs">
              {t('password.autoLogout.enableLabel')}
            </Label>
            <TypographyText size="xs" tone="muted" className="mt-1">
              {t('password.autoLogout.enableHint')}
            </TypographyText>
          </div>
          <Switch
            id="auto-logout-enabled"
            checked={enabled}
            onCheckedChange={setEnabled}
            disabled={save.isPending}
            aria-label={t('password.autoLogout.enableLabel')}
          />
        </Flex>

        <div>
          <Label htmlFor="auto-logout-seconds" className="mb-1.5 text-xs">
            {t('password.autoLogout.fieldLabel')}
          </Label>
          <NumberInput
            id="auto-logout-seconds"
            nullable
            allowOutOfRange
            min={MIN_IDLE_SECONDS}
            max={MAX_IDLE_SECONDS}
            step={1}
            value={toNumberOrNull(seconds)}
            onChange={(next) => setSeconds(toNumberText(next))}
            disabled={save.isPending || !enabled}
          />
          <TypographyText size="xs" tone="muted" className="mt-1">
            {showsEquivalent
              ? t('password.autoLogout.equivalent', {
                  minutes: Math.round(parsedSeconds / 60),
                })
              : t('password.autoLogout.hint', {
                  min: MIN_IDLE_SECONDS,
                  max: MAX_IDLE_SECONDS,
                })}
          </TypographyText>
        </div>

        {error && (
          <div className="rounded-md border border-destructive/50 bg-destructive/10 p-3 text-xs text-destructive">
            {error}
          </div>
        )}

        <Button type="submit" permission={PERM.SETTINGS_MANAGE} disabled={save.isPending}>
          {save.isPending ? t('common:actions.saving') : t('common:actions.save')}
        </Button>
      </form>
    </div>
  );
}
