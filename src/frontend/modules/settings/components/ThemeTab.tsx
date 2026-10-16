// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file:
//   settings:themeTab.tabHint
//   settings:themeTab.section.{variant,variables,reset}

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconRefresh, IconSun, IconMoon } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { THEME_VARS, useThemeCustomizerStore, type ThemeVarOverride } from '@resources/stores/themeCustomizer';
import { useThemeStore } from '@resources/stores/theme';
import { useThemeTokenVersion } from '@resources/theme/token-version';
import { getTheme } from '../../../src/themes';
import { Flex, Stack } from '@resources/components/ui/Layout';
import { Grid } from '@resources/components/ui';
import { SettingsShell } from '@resources/components/ui/SettingsShell';
import { Section } from '@resources/components/ui/Form';
import { AppearanceDefaultsCard } from './AppearanceDefaultsCard';
import { BrandingCard } from './BrandingCard';
import { MotionPreference } from './MotionPreference';
import { VariableOverridesBody, useFilteredVariables } from './ThemeVariableOverrides';

export function ThemeTab() {
  const { t } = useTranslation('settings');
  // Theme, accent, font and base size are governed now - the app-wide default
  // and each account's override both live on the server, and
  // `AppearanceDefaultsCard` owns that half of this tab. What is left here is
  // the local-only power tools: light/dark, motion, and the per-variable
  // colour overrides keyed by theme+variant.
  const { themeId, variant, overrides, setOverride, resetOverrides, reset } = useThemeCustomizerStore();
  // Light/dark lives in `useThemeStore` - it is the same switch as the one
  // in the topbar, and pointing both at one store is what stopped them
  // from undoing each other.
  const setMode = useThemeStore((s) => s.setTheme);

  const overridesKey = JSON.stringify(overrides);
  const [activeGroup, setActiveGroup] = useState<string>('all');
  const [refreshTick, setRefreshTick] = useState(0);
  // The rows below read their values off <html> with getComputedStyle,
  // which React cannot invalidate. This re-renders them the moment the
  // theme's variables are actually rewritten - a theme stylesheet lands
  // asynchronously, so rendering on `themeId` alone read the old sheet.
  const tokenVersion = useThemeTokenVersion();

  const liveOverrides = useMemo<ThemeVarOverride>(() => {
    void refreshTick;
    void overridesKey;
    const key = `${themeId}:${variant}`;
    return overrides[key] ?? ({} as ThemeVarOverride);
  }, [themeId, variant, overrides, refreshTick, overridesKey]);

  const active = getTheme(themeId);
  const filteredVars = useFilteredVariables(activeGroup);

  const overriddenCount = Object.values(liveOverrides).filter(Boolean).length;

  return (
    <SettingsShell title={t('tabs.themes')} description={t('themeTab.tabHint')}>
      <Stack gap={4}>
        {/* The install's own mark leads the tab: it is the first thing an
            operator changes when they make McHarbor theirs, and it is the one
            setting on this page a visitor sees before signing in. */}
        <BrandingCard />

        {/* Theme, accent, font and base font size - the governed set. The card
            carries its own operator/personal split, so this tab does not need
            a section wrapper around it. */}
        <AppearanceDefaultsCard />

        {/* Variant + Reset side-by-side */}
        <Grid columns={2} gap={4} className="grid-cols-1 lg:grid-cols-2">
          <Section title={t('themeTab.section.variant')}>
            <Flex gap={2}>
              <Button
                type="button"
                variant={variant === 'light' ? 'default' : 'outline'}
                size="sm"
                onClick={() => setMode('light')}
                className="flex items-center gap-2"
              >
                <IconSun className="size-4" />
                {t('themeTab.light')}
              </Button>
              <Button
                type="button"
                variant={variant === 'dark' ? 'default' : 'outline'}
                size="sm"
                onClick={() => setMode('dark')}
                className="flex items-center gap-2"
              >
                <IconMoon className="size-4" />
                {t('themeTab.dark')}
              </Button>
            </Flex>
          </Section>

          <Section title={t('motion.section')} help={t('motion.help')}>
            <MotionPreference />
          </Section>

          <Section title={t('themeTab.section.reset')} help={t('themeTab.themeName', { name: active.name })}>
            <Flex justify="end">
              <Button variant="outline" size="sm" onClick={reset}>
                <IconRefresh className="size-3.5" />
                {t('themeTab.resetAll')}
              </Button>
            </Flex>
          </Section>
        </Grid>

        {/* CSS variables — full width with internal 2-col override grid */}
        <Section
          icon={<IconRefresh className="size-3.5" />}
          title={t('themeTab.section.variables')}
          help={`${overriddenCount} / ${THEME_VARS.length} ${t('themeTab.overridden')}`}
        >
          <VariableOverridesBody
            activeGroup={activeGroup}
            setActiveGroup={setActiveGroup}
            filteredVars={filteredVars}
            liveOverrides={liveOverrides}
            tokenVersion={tokenVersion}
            overriddenCount={overriddenCount}
            onSetOverride={(varName, v) => {
              setOverride(varName, v);
              setRefreshTick((tick) => tick + 1);
            }}
            onClearOverride={(varName) => {
              setOverride(varName, '');
              setRefreshTick((tick) => tick + 1);
            }}
            onResetOverrides={resetOverrides}
          />
        </Section>
      </Stack>
    </SettingsShell>
  );
}

export default ThemeTab;
