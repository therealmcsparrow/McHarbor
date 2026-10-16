// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { cssVars } from '@resources/utils/cssVars';
import { PlainButton } from '@resources/components/ui/PlainButton';
import { Box } from '@resources/components/ui/Layout';
import { Caption } from '@resources/components/ui/Typography';

type Props = {
  label: string;
  track: string;
  thumbPercent: number;
  onPick: (ratio: number) => void;
};

export function ThemeColorSlider({ label, track, thumbPercent, onPick }: Props) {
  return (
    <Box className="space-y-1">
      <Caption>{label}</Caption>
      <PlainButton
        aria-label={label}
        className="relative block h-3 w-full cursor-pointer overflow-hidden rounded-full border border-border bg-(image:--track)"
        style={cssVars({ track })}
        onClick={(e) => {
          const rect = e.currentTarget.getBoundingClientRect();
          onPick((e.clientX - rect.left) / rect.width);
        }}
      >
        <span
          className="absolute top-0 left-(--x) size-3 -translate-x-1/2 -translate-y-px rounded-full border-2 border-foreground bg-background"
          style={cssVars({ x: `${thumbPercent}%` })}
        />
      </PlainButton>
    </Box>
  );
}
