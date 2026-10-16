// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { PlainButton } from '@resources/components/ui/PlainButton';
import { useTranslation } from 'react-i18next';
import { IconRefresh } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { cn } from '@resources/utils/cn';
import { THEME_VARS, type ThemeVarName, type ThemeVarOverride } from '@resources/stores/themeCustomizer';
import { Box, Flex } from '@resources/components/ui/Layout';
import { Grid } from '@resources/components/ui';
import { Text as BodyText } from '@resources/components/ui/Typography';
import { ThemeColorPicker } from './ThemeColorPicker';

const VARIABLE_GROUPS: Array<{
  label: string;
  vars: readonly ThemeVarName[];
}> = [
  {
    label: 'Surface',
    vars: ['background', 'foreground', 'card', 'card-foreground', 'popover', 'popover-foreground'] as const,
  },
  {
    label: 'Brand & Actions',
    vars: [
      'primary',
      'primary-foreground',
      'secondary',
      'secondary-foreground',
      'destructive',
      'destructive-foreground',
    ] as const,
  },
  {
    label: 'Muted & Accent',
    vars: ['muted', 'muted-foreground', 'accent', 'accent-foreground', 'ring'] as const,
  },
  {
    label: 'Borders & Inputs',
    vars: ['border', 'input'] as const,
  },
  {
    label: 'Sidebar',
    vars: [
      'sidebar',
      'sidebar-foreground',
      'sidebar-primary',
      'sidebar-primary-foreground',
      'sidebar-accent',
      'sidebar-accent-foreground',
      'sidebar-border',
      'sidebar-ring',
    ] as const,
  },
];

/**
 * Read the *live* value of a CSS custom property on the document root.
 * The value reflects the active theme stylesheet + any inline overrides
 * the user has applied, so the swatch always shows the real rendered
 * color.
 */
function readVar(varName: ThemeVarName): string {
  if (typeof window === 'undefined') return '';
  return getComputedStyle(document.documentElement).getPropertyValue(`--${varName}`).trim();
}

// useFilteredVariables returns the variable list for the selected
// "group" filter, or the full list when "all" is selected. Extracted
// so ThemeTab stays focused on state setup.
export function useFilteredVariables(activeGroup: string) {
  if (activeGroup === 'all') {
    return THEME_VARS;
  }
  return VARIABLE_GROUPS.find((g) => g.label === activeGroup)?.vars ?? THEME_VARS;
}

// VariableOverridesBody is the body content of the "CSS variables"
// section: the group filter, the variable grid, and the reset
// overrides action.
export function VariableOverridesBody({
  activeGroup,
  setActiveGroup,
  filteredVars,
  liveOverrides,
  tokenVersion,
  overriddenCount,
  onSetOverride,
  onClearOverride,
  onResetOverrides,
}: {
  activeGroup: string;
  setActiveGroup: (g: string) => void;
  filteredVars: readonly ThemeVarName[];
  liveOverrides: ThemeVarOverride;
  tokenVersion: number;
  overriddenCount: number;
  onSetOverride: (name: ThemeVarName, value: string) => void;
  onClearOverride: (name: ThemeVarName) => void;
  onResetOverrides: () => void;
}) {
  const { t } = useTranslation('settings');
  return (
    <Box className="space-y-3">
      <BodyText variant="muted" size="sm">
        {t('themeTab.variablesHint')}
      </BodyText>

      <GroupFilterRow activeGroup={activeGroup} onPickGroup={setActiveGroup} />
      <Grid columns={2} gap={2} className="grid-cols-1 sm:grid-cols-2">
        {filteredVars.map((varName) => (
          <VariableOverrideRow
            key={varName}
            name={varName}
            liveOverrides={liveOverrides}
            tokenVersion={tokenVersion}
            onSetOverride={onSetOverride}
            onClearOverride={onClearOverride}
          />
        ))}
      </Grid>
      {overriddenCount > 0 && (
        <Flex justify="end">
          <Button variant="ghost" size="sm" onClick={onResetOverrides}>
            <IconRefresh className="size-3.5" />
            {t('themeTab.resetVars')}
          </Button>
        </Flex>
      )}
    </Box>
  );
}

// GroupFilterRow is the "All / Surface / Brand & Actions / ..." chip
// row that filters which variable overrides are shown.
function GroupFilterRow({ activeGroup, onPickGroup }: { activeGroup: string; onPickGroup: (g: string) => void }) {
  const { t } = useTranslation('settings');
  return (
    <Flex gap={1} wrap>
      <PlainButton
        type="button"
        onClick={() => onPickGroup('all')}
        className={cn(
          'mc-control rounded-md px-3 text-xs font-medium transition-colors',
          activeGroup === 'all'
            ? 'bg-primary text-primary-foreground'
            : 'text-muted-foreground hover:bg-muted/50 hover:text-foreground',
        )}
      >
        {t('themeTab.all')}
      </PlainButton>
      {VARIABLE_GROUPS.map((group) => (
        <PlainButton
          key={group.label}
          type="button"
          onClick={() => onPickGroup(group.label)}
          className={cn(
            'mc-control rounded-md px-3 text-xs font-medium transition-colors',
            group.label === activeGroup
              ? 'bg-primary text-primary-foreground'
              : 'text-muted-foreground hover:bg-muted/50 hover:text-foreground',
          )}
        >
          {group.label}
        </PlainButton>
      ))}
    </Flex>
  );
}

// VariableOverrideRow is a single variable entry: its CSS name, its
// current resolved value, and a color picker for editing.
function VariableOverrideRow({
  name: varName,
  liveOverrides,
  tokenVersion,
  onSetOverride,
  onClearOverride,
}: {
  name: ThemeVarName;
  liveOverrides: ThemeVarOverride;
  /** Read only to re-run `readVar` when the document's tokens change. */
  tokenVersion: number;
  onSetOverride: (name: ThemeVarName, value: string) => void;
  onClearOverride: (name: ThemeVarName) => void;
}) {
  void tokenVersion;
  const { t } = useTranslation('settings');
  const liveValue = readVar(varName);
  const override = liveOverrides[varName] ?? '';
  return (
    <Flex align="center" justify="between" gap={2} className="mc-glass rounded-md p-2">
      <Box className="min-w-0 flex-1">
        <BodyText size="xs" className="truncate font-mono">
          --{varName}
        </BodyText>
        <BodyText variant="muted" size="xs" className="truncate font-mono">
          {override || liveValue || t('themeTab.themeDefault')}
        </BodyText>
      </Box>
      <ThemeColorPicker
        name={varName}
        value={override}
        liveValue={liveValue}
        onChange={(v) => onSetOverride(varName, v)}
        onReset={() => onClearOverride(varName)}
      />
    </Flex>
  );
}
