// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { Card, CardContent, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { InfoRow } from '@resources/components/ui/InfoRow';
import { Stack } from '@resources/components/ui/Layout';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { formatDate, formatDateOnly } from '@resources/utils/format';
import { UNLIMITED_SERVERS, type LicenseRecord, type LicenseState, type LicenseStatus } from '../types';

const STATE_TONE: Record<LicenseState, 'success' | 'warning' | 'destructive' | 'secondary'> = {
  ok: 'success',
  over: 'warning',
  expired: 'destructive',
  unlicensed: 'secondary',
  suspended: 'warning',
  revoked: 'destructive',
  pending_approval: 'warning',
  not_started: 'secondary',
};

type LicenseStatusCardProps = {
  status: LicenseStatus;
  license: LicenseRecord | null;
  serverConfigured: boolean;
};

export function LicenseStatusCard({ status: currentStatus, license, serverConfigured }: LicenseStatusCardProps) {
  const { t } = useTranslation('license');

  // Three shapes, because there are three kinds of ceiling: none at all
  // (the unmetered licences), a number, and no entitlement whatsoever (no
  // licence). "2 of 0" is not a sentence anyone should have to read.
  const seats =
    currentStatus.serversLimit === UNLIMITED_SERVERS
      ? t('status.serversUnlimited', { used: currentStatus.serversUsed })
      : currentStatus.serversLimit === 0
        ? t('status.serversNoLicense', { used: currentStatus.serversUsed })
        : t('status.serversValue', { used: currentStatus.serversUsed, limit: currentStatus.serversLimit });

  // Days remaining is the number that turns a dead trial into a renewal
  // conversation, so it sits next to the expiry rather than in a tooltip.
  const expiry = license?.term.perpetual
    ? t('status.neverExpires')
    : currentStatus.expiresAt
      ? currentStatus.daysRemaining > 0
        ? t('status.expiresValue', {
            date: formatDateOnly(currentStatus.expiresAt),
            count: currentStatus.daysRemaining,
          })
        : formatDateOnly(currentStatus.expiresAt)
      : t('status.neverExpires');

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('status.title')}</CardTitle>
      </CardHeader>
      <CardContent>
        <Stack gap={2}>
          <InfoRow label={t('status.tier')}>
            {/* eslint-disable-next-line mcharbor/no-restricted-syntax -- i18n-open-set: license tier and reason codes are backend strings and can be extended without a frontend release */}
            {t(`tier.${currentStatus.tier}`, { defaultValue: currentStatus.tier })}
          </InfoRow>
          <InfoRow label={t('status.state')}>
            <Badge variant={STATE_TONE[currentStatus.state]}>{t(`state.${currentStatus.state}`)}</Badge>
          </InfoRow>
          <InfoRow label={t('status.servers')}>{seats}</InfoRow>
          <InfoRow label={t('status.expires')}>{expiry}</InfoRow>
          {license && (
            <>
              <InfoRow label={t('status.key')}>{license.keyMasked}</InfoRow>
              <InfoRow label={t('status.seat')}>
                {license.holdsSeat ? t('status.seatHeld') : t('status.seatLost')}
              </InfoRow>
              <InfoRow label={t('status.autoRenew')}>
                {license.term.autoRenew ? t('yes', { ns: 'common' }) : t('no', { ns: 'common' })}
              </InfoRow>
              <InfoRow label={t('status.lastVerified')}>
                {license.lastCheckedAt ? formatDate(license.lastCheckedAt) : t('status.never')}
              </InfoRow>
              {!license.valid && license.reason && (
                <InfoRow label={t('status.reason')}>
                  <TypographyText tone="warning" as="span" size="sm">
                    {/* eslint-disable-next-line mcharbor/no-restricted-syntax -- i18n-open-set: license tier and reason codes are backend strings and can be extended without a frontend release */}
                    {t(`reason.${license.reason}`, { defaultValue: license.reason })}
                  </TypographyText>
                </InfoRow>
              )}
              {license.lastError && (
                <InfoRow label={t('status.lastError')}>
                  <TypographyText tone="warning" as="span" size="sm">
                    {t(`lastErrorReason.${license.lastError}`, {
                      // eslint-disable-next-line mcharbor/no-restricted-syntax -- i18n-open-set: license tier and reason codes are backend strings and can be extended without a frontend release
                      defaultValue: t('lastErrorReason.unknown'),
                    })}
                  </TypographyText>
                </InfoRow>
              )}
            </>
          )}
        </Stack>

        {!serverConfigured && (
          <TypographyText tone="muted" size="xs" className="mt-3 block">
            {t('status.serverNotConfigured')}
          </TypographyText>
        )}

        {currentStatus.offlineDays > 0 && (
          <TypographyText tone="muted" size="xs" className="mt-3 block">
            {t('status.offline', { count: currentStatus.offlineDays })}
          </TypographyText>
        )}
      </CardContent>
    </Card>
  );
}
