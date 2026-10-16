// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { Button } from './Button';

// Ladle uses file-level `Meta` and named exports as "stories".
// Each export is rendered in its own tab in the catalogue UI.
export default {
  title: 'Primitives / Button',
};

// Every variant × size combination — one button per cell.
export const Variants = () => (
  <div className="flex flex-col gap-6 p-8">
    {(['default', 'destructive', 'outline', 'secondary', 'ghost', 'link'] as const).map((variant) => (
      <div key={variant} className="flex items-center gap-4">
        <span className="w-32 text-xs text-muted-foreground uppercase">{variant}</span>
        <Button variant={variant}>Default</Button>
        <Button variant={variant} size="sm">
          Small
        </Button>
        <Button variant={variant} size="lg">
          Large
        </Button>
        <Button variant={variant} disabled>
          Disabled
        </Button>
      </div>
    ))}
  </div>
);

// Icon-only sizes — make sure the `aria-label` is always set in
// real usage. The empty tooltip here is just for visual reference.
export const IconButtons = () => (
  <div className="flex items-center gap-4 p-8">
    <Button size="icon" aria-label="Default icon button">
      ★
    </Button>
    <Button size="icon-sm" aria-label="Small icon button">
      ★
    </Button>
    <Button variant="outline" size="icon" aria-label="Outline icon">
      ★
    </Button>
    <Button variant="destructive" size="icon" aria-label="Destructive icon">
      ★
    </Button>
    <Button variant="ghost" size="icon" aria-label="Ghost icon">
      ★
    </Button>
  </div>
);
