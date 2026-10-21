// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// LicenseBanner — the entire user-facing half of McHarbor's licensing.
//
// Three properties, and all three are load-bearing:
//
//   1. It is visible for exactly as long as the condition stands. Anything
//      other than a healthy licence — unlicensed, suspended, revoked,
//      expired, over seats, pending, not yet started — pins the band open.
//      A billing state nobody can see is a billing state nobody fixes, so
//      there is no dismissing it into a chip.
//   2. Its ledger cadence (daily for the first fourteen days, hourly after)
//      is decided by the server. The browser only reports that a showing
//      happened, so a reload cannot re-trigger one and two open tabs
//      cannot double-count. The cadence governs the evidence trail only,
//      never whether the band is on screen.
//   3. It never blocks anything, and every string says what still works
//      before it says what is wrong.

import { useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';
import { IconAlertTriangle } from '@tabler/icons-react';
import { DockableBanner } from '@resources/layout/DockableBanner';
import { Button } from '@resources/components/ui/Button';
import { useDockedBanners } from '@resources/stores/docked-banners';
import { formatDateOnly } from '@resources/utils/format';
import { useAcknowledgeNotice, useLicenseStatus } from '../hooks/useLicense';
import { UNLIMITED_SERVERS, type LicenseNoticeKind } from '../types';

/** Stable across releases — the dock store still keys off it. */
const BANNER_ID = 'license-notice';

/** How long a showing must stay in front of someone before it is recorded. */
const VISIBLE_MS = 30_000;

const TONE_BY_KIND: Record<LicenseNoticeKind, 'warning' | 'danger'> = {
  overage: 'warning',
  unlicensed: 'warning',
  expired: 'danger',
  // Suspension and revocation are billing events, not outages — but a
  // revoked key is the one condition nothing local can resolve, so it
  // reads as loudly as an expiry.
  suspended: 'warning',
  revoked: 'danger',
  pending_approval: 'warning',
  not_started: 'warning',
};

type LicenseBannerProps = {
  /** Placement, supplied by whatever mounts it. */
  className?: string;
};

export function LicenseBanner({ className }: LicenseBannerProps = {}) {
  const { t } = useTranslation('license');
  const { data: currentStatus } = useLicenseStatus();
  const { mutate: acknowledgeKind } = useAcknowledgeNotice();
  const restore = useDockedBanners((s) => s.restore);

  // The condition, not the cadence, decides what is on screen. The server
  // sets a kind for every state that is not a healthy licence.
  const kind = currentStatus?.noticeKind;
  const due = Boolean(currentStatus?.noticeDue);

  // The dock store persists, so a banner docked by an earlier build would
  // suppress this one forever. Undock on every showing — nothing about a
  // standing licence condition is dismissible any more.
  //
  // The acknowledgement fires at the *end* of the 30 seconds, not at the
  // start, and the 30 seconds only run while the tab is actually in front
  // of someone. Acknowledging on render meant a two-second visit — or a
  // background tab — burned the day's ledger entry on a notice nobody read.
  // Leaving early now simply leaves it due, so it is recorded on the next
  // page instead.
  const startedFor = useRef<string | null>(null);
  useEffect(() => {
    if (!kind) return;

    restore(BANNER_ID);
    if (!due) return;

    const showing = `${kind}:${currentStatus?.sampledAt ?? ''}`;
    if (startedFor.current === showing) return;

    let timer: ReturnType<typeof setTimeout> | undefined;

    const begin = () => {
      startedFor.current = showing;
      timer = setTimeout(() => {
        acknowledgeKind(kind);
      }, VISIBLE_MS);
    };

    if (document.visibilityState === 'visible') {
      begin();
      return () => clearTimeout(timer);
    }

    const onVisible = () => {
      if (document.visibilityState === 'visible') {
        document.removeEventListener('visibilitychange', onVisible);
        begin();
      }
    };
    document.addEventListener('visibilitychange', onVisible);
    return () => {
      document.removeEventListener('visibilitychange', onVisible);
      clearTimeout(timer);
    };
  }, [kind, due, currentStatus?.sampledAt, restore, acknowledgeKind]);

  if (!currentStatus || !kind) return null;

  const limitLabel = currentStatus.serversLimit === UNLIMITED_SERVERS ? '∞' : String(currentStatus.serversLimit);

  // Every kind gets its own sentence. The interpolations differ, so this
  // is a switch rather than one key with optional values — a banner that
  // says "{{used}} of {{limit}}" about a suspension is worse than no
  // banner at all.
  const message =
    kind === 'expired'
      ? t('banner.expired', {
          tier: t(`tier.${currentStatus.tier}`),
          date: formatDateOnly(currentStatus.expiresAt),
        })
      : kind === 'unlicensed'
        ? t('banner.unlicensed', { used: currentStatus.serversUsed })
        : kind === 'overage'
          ? t('banner.overage', { used: currentStatus.serversUsed, limit: limitLabel })
          : t(`banner.${kind}`);

  const hint =
    kind === 'expired'
      ? t('banner.expiredHint')
      : kind === 'unlicensed'
        ? t('banner.unlicensedHint')
        : kind === 'overage'
          ? t('banner.overageHint')
          : t(`banner.${kind}Hint`);

  return (
    <DockableBanner
      id={BANNER_ID}
      label={t('banner.chip')}
      tone={TONE_BY_KIND[kind]}
      icon={<IconAlertTriangle className="size-4" />}
      dockable={false}
      persistent
      className={className}
    >
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <span className="font-medium text-foreground">{message}</span>
        <span className="text-muted-foreground">{hint}</span>
        {currentStatus.offlineDays > 0 && (
          <span className="text-muted-foreground">{t('banner.offline', { count: currentStatus.offlineDays })}</span>
        )}
        {/* The licence tab is where every one of these notices is actually
            resolved — a key entered, a term inspected, a contact link
            followed — so the banner points there rather than off-site. */}
        <Button variant="outline" size="sm" asChild>
          <Link to="/settings?tab=license">{t('banner.manage')}</Link>
        </Button>
      </div>
    </DockableBanner>
  );
}
