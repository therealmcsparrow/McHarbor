// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { cssVars } from '@resources/utils/cssVars';
import { PlainButton } from '@resources/components/ui/PlainButton';
import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Popover, PopoverContent, PopoverTrigger } from '@resources/components/ui/Popover';
import { Box, Flex } from '@resources/components/ui/Layout';
import { Text as BodyText, Caption } from '@resources/components/ui/Typography';
import { Button } from '@resources/components/ui/Button';
import { IconRefresh } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';
import { ThemeColorSlider } from './ThemeColorSlider';

import { ensureColor, formatOklch, hexToOklch, oklchToHex, parseOklch } from '@resources/theme/oklch';

// The colour maths lives in `@resources/theme/oklch` because
// `useApplyThemeTokens` derives the aurora and the accent wash from the
// same primary this picker writes, and the two must agree exactly.
// Re-exported here so existing importers of the picker keep working.
export { ensureColor, hexToOklch, parseOklch };

/* ------------------------------------------------------------------ *
 * Component
 * ------------------------------------------------------------------ */

type Props = {
  name: string;
  value: string;
  liveValue: string;
  onChange: (oklch: string) => void;
  onReset: () => void;
};

export function ThemeColorPicker({ name: colorName, value, liveValue, onChange, onReset }: Props) {
  const { t } = useTranslation('settings');
  const activeValue = value || liveValue;
  const initial = parseOklch(activeValue);
  const [L, setL] = useState<number>(initial?.[0] ?? 0.5);
  const [C, setC] = useState<number>(initial?.[1] ?? 0.1);
  const [h, setH] = useState<number>(initial?.[2] ?? 250);
  const [hex, setHex] = useState<string>(() => oklchToHex(L, C, h));
  const [input, setInput] = useState<string>(activeValue);
  const [isOpen, setOpen] = useState(false);
  const lastEmitted = useRef<string>('');

  const syncKey = `${activeValue}|${isOpen}`;
  const [syncedKey, setSyncedKey] = useState(syncKey);
  if (syncKey !== syncedKey) {
    setSyncedKey(syncKey);
    const parsed = isOpen ? null : parseOklch(activeValue);
    if (parsed) {
      setL(parsed[0]);
      setC(parsed[1]);
      setH(parsed[2]);
      setHex(oklchToHex(parsed[0], parsed[1], parsed[2]));
      setInput(activeValue);
    }
  }

  function push(newL: number, newC: number, newH: number) {
    setL(newL);
    setC(newC);
    setH(newH);
    const newHex = oklchToHex(newL, newC, newH);
    setHex(newHex);
    const next = formatOklch(newL, newC, newH);
    setInput(next);
    if (next !== lastEmitted.current) {
      lastEmitted.current = next;
      onChange(next);
    }
  }

  function commitInputText(raw: string) {
    setInput(raw);
    const parsed = parseOklch(raw);
    if (parsed) {
      push(parsed[0], parsed[1], parsed[2]);
    }
  }

  const swatchColor = oklchToHex(L, C, h);
  const isCustom = value !== '';

  return (
    <Popover open={isOpen} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <PlainButton
          type="button"
          className={cn(
            'mc-control mc-glass flex shrink-0 cursor-pointer items-center gap-2 rounded px-2 transition-colors hover:bg-muted/30',
          )}
          aria-label={t('theme.picker.edit', { name: colorName })}
        >
          <span
            className="size-5 shrink-0 rounded border border-border bg-(--swatch)"
            style={cssVars({ swatch: swatchColor })}
          />
          {isCustom && <span className="text-xs text-primary">{t('theme.picker.custom')}</span>}
        </PlainButton>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        sideOffset={8}
        className="z-50 w-72 rounded-lg border border-border bg-popover p-4 text-popover-foreground shadow-xl"
      >
        <Box className="space-y-3">
          <Flex align="center" justify="between">
            <BodyText size="sm" className="font-mono font-semibold">
              --{colorName}
            </BodyText>
            {isCustom && (
              <PlainButton
                type="button"
                onClick={onReset}
                className="text-xs text-primary hover:underline"
                aria-label={t('general.themeResetLabel')}
              >
                <IconRefresh className="size-3.5" />
              </PlainButton>
            )}
          </Flex>

          <div
            className="h-16 w-full rounded-md border border-border bg-(--swatch)"
            style={cssVars({ swatch: swatchColor })}
          />

          <ThemeColorSlider
            label={t('theme.oklch.hue')}
            track="linear-gradient(to right, oklch(0.7 0.2 0), oklch(0.7 0.2 60), oklch(0.7 0.2 120), oklch(0.7 0.2 180), oklch(0.7 0.2 240), oklch(0.7 0.2 300), oklch(0.7 0.2 360))"
            thumbPercent={(h / 360) * 100}
            onPick={(ratio) => push(L, C, Math.max(0, Math.min(360, ratio * 360)))}
          />

          <ThemeColorSlider
            label={t('theme.oklch.lightness')}
            track={`linear-gradient(to right, oklch(0 0 ${h}), oklch(1 0 ${h}))`}
            thumbPercent={L * 100}
            onPick={(ratio) => push(Math.max(0, Math.min(1, ratio)), C, h)}
          />

          <ThemeColorSlider
            label={t('theme.oklch.chroma')}
            track={`linear-gradient(to right, oklch(${L} 0 ${h}), oklch(${L} 0.4 ${h}))`}
            thumbPercent={Math.min(100, (C / 0.4) * 100)}
            onPick={(ratio) => push(L, Math.max(0, Math.min(0.4, ratio * 0.4)), h)}
          />

          <Box className="space-y-1">
            <Caption>{t('theme.picker.oklchLabel')}</Caption>
            <input
              type="text"
              value={input}
              onChange={(e) => commitInputText(e.target.value)}
              spellCheck={false}
              className="mc-control mc-glass w-full rounded-md px-2 font-mono text-xs focus:border-primary focus:outline-none"
            />
          </Box>

          <Flex align="center" justify="between">
            <Caption>{t('theme.picker.hexLabel')}</Caption>
            <BodyText size="xs" className="font-mono">
              {hex}
            </BodyText>
          </Flex>

          <Flex align="center" justify="end" gap={2} className="pt-1">
            <Button variant="outline" size="sm" onClick={() => setOpen(false)}>
              {t('actions.close', { ns: 'common' })}
            </Button>
            {isCustom && (
              <Button
                size="sm"
                onClick={() => {
                  onReset();
                  setOpen(false);
                }}
              >
                {t('theme.picker.reset')}
              </Button>
            )}
          </Flex>
        </Box>
      </PopoverContent>
    </Popover>
  );
}
