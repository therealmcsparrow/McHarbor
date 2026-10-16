// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// i18n keys used by this file:
//   settings:branding.{title,description,readOnlyTitle}
//   settings:branding.{wideHeading,iconHeading}
//   settings:branding.{lightLabel,darkLabel}
//   settings:branding.{wideLightDescription,wideDarkDescription}
//   settings:branding.{iconLightDescription,iconDarkDescription}

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPhoto } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { usePermissionGate } from '@resources/hooks/usePermissionGate';
import { PERM } from '@core/auth/permissionKeys';
import {
  BRANDING_EMPTY,
  BRANDING_FALLBACK,
  resolveBrandMark,
  useBranding,
  useSaveBranding,
  type BrandingSettings,
} from '@resources/hooks/useBranding';
import { SettingsGroup } from '@resources/components/settings/SettingsGroup';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { BrandingLogoField } from './BrandingLogoField';

/**
 * The application's own mark — what the sidebar and the login page render
 * instead of the McHarbor logo.
 *
 * Four slots: two marks × two variants. The wordmark and the icon are two
 * marks because the collapsed sidebar rail is 56px wide and cannot fit a
 * lockup; light and dark are two variants because a mark drawn for a white
 * page frequently disappears on the dark rail.
 *
 * The dark slot is an *override*, not a requirement — leaving it empty falls
 * back to the light upload, which falls back to the bundled McHarbor mark. So
 * an install that has one usable logo uploads it once and is done.
 *
 * Like `AppearanceDefaultsCard` this renders for everyone and disables rather
 * than hides, so a user without `settings.manage` can see the capability
 * exists and which permission would unlock it.
 */
export function BrandingCard() {
  const { t } = useTranslation('settings');
  const { t: tc } = useTranslation('common');
  const { data: branding } = useBranding();
  const saveBranding = useSaveBranding();
  const saveMutate = saveBranding.mutate;
  const { allowed: canManage, reason: permissionReason } = usePermissionGate(PERM.SETTINGS_MANAGE);

  const [draft, setDraft] = useState<BrandingSettings>(BRANDING_EMPTY);

  const [syncedBranding, setSyncedBranding] = useState<typeof branding>(undefined);
  if (branding !== syncedBranding) {
    setSyncedBranding(branding);
    if (branding) setDraft(branding);
  }

  function set<K extends keyof BrandingSettings>(key: K, value: string) {
    setDraft((prev) => ({ ...prev, [key]: value }));
  }

  /**
   * A light slot also carries the PNG rendition document exporters read, so
   * the two move together — clearing the mark clears its rendition, and
   * replacing the mark replaces it.
   */
  function setLight(key: 'logoWide' | 'logoIcon', rasterKey: 'logoWidePng' | 'logoIconPng') {
    return (value: string, raster: string) => setDraft((prev) => ({ ...prev, [key]: value, [rasterKey]: raster }));
  }

  return (
    <SettingsGroup
      icon={IconPhoto}
      title={t('branding.title')}
      description={t('branding.description')}
      scope={t('general.scopeGlobal')}
      actions={
        <Button
          permission={PERM.SETTINGS_MANAGE}
          onClick={() => saveMutate(draft)}
          disabled={saveBranding.isPending || !branding}
        >
          {saveBranding.isPending ? t('general.saving') : tc('actions.save')}
        </Button>
      }
    >
      {!canManage && <Alert variant="info" title={t('branding.readOnlyTitle')} description={permissionReason} />}

      <div className="space-y-4">
        <TypographyText variant="small" className="font-semibold text-foreground">
          {t('branding.wideHeading')}
        </TypographyText>
        <div className="grid grid-cols-1 gap-5 lg:grid-cols-2">
          <BrandingLogoField
            label={t('branding.lightLabel')}
            description={t('branding.wideLightDescription')}
            value={draft.logoWide}
            // The light slot previews its own upload or the bundled mark; the
            // dark slot previews what dark mode would *actually* render,
            // which is the light upload whenever the dark slot is empty.
            fallbackSrc={BRANDING_FALLBACK.wide}
            onChange={setLight('logoWide', 'logoWidePng')}
            withRaster
            disabled={!canManage}
            permission={PERM.SETTINGS_MANAGE}
            previewClassName="h-16 w-full max-w-[14rem]"
            ground="light"
          />
          <BrandingLogoField
            label={t('branding.darkLabel')}
            description={t('branding.wideDarkDescription')}
            value={draft.logoWideDark}
            fallbackSrc={resolveBrandMark(draft, 'wide', 'dark') || BRANDING_FALLBACK.wide}
            onChange={(value) => set('logoWideDark', value)}
            disabled={!canManage}
            permission={PERM.SETTINGS_MANAGE}
            previewClassName="h-16 w-full max-w-[14rem]"
            ground="dark"
          />
        </div>
      </div>

      <div className="space-y-4">
        <TypographyText variant="small" className="font-semibold text-foreground">
          {t('branding.iconHeading')}
        </TypographyText>
        <div className="grid grid-cols-1 gap-5 lg:grid-cols-2">
          <BrandingLogoField
            label={t('branding.lightLabel')}
            description={t('branding.iconLightDescription')}
            value={draft.logoIcon}
            fallbackSrc={BRANDING_FALLBACK.icon}
            onChange={setLight('logoIcon', 'logoIconPng')}
            withRaster
            disabled={!canManage}
            permission={PERM.SETTINGS_MANAGE}
            previewClassName="size-16"
            ground="light"
          />
          <BrandingLogoField
            label={t('branding.darkLabel')}
            description={t('branding.iconDarkDescription')}
            value={draft.logoIconDark}
            fallbackSrc={resolveBrandMark(draft, 'icon', 'dark') || BRANDING_FALLBACK.icon}
            onChange={(value) => set('logoIconDark', value)}
            disabled={!canManage}
            permission={PERM.SETTINGS_MANAGE}
            previewClassName="size-16"
            ground="dark"
          />
        </div>
      </div>
    </SettingsGroup>
  );
}
