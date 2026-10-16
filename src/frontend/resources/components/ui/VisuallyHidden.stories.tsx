// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { VisuallyHidden } from './VisuallyHidden';
import { Box } from './Layout';

export default {
  title: 'Primitives / VisuallyHidden',
};

export const Basic = () => (
  <Box className="p-8">
    <p className="text-sm text-foreground">
      The text after this sentence is invisible to sighted users but still announced by screen readers:{' '}
      <VisuallyHidden>(status: running, uptime 3 days)</VisuallyHidden>.
    </p>
  </Box>
);

export const OnIconButton = () => (
  <Box className="p-8">
    <button type="button" className="mc-glass rounded-md p-2 hover:bg-muted/50">
      ⚙<VisuallyHidden>Open settings</VisuallyHidden>
    </button>
  </Box>
);
