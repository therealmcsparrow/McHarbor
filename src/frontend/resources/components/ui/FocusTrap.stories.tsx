// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { useState } from 'react';
import { FocusTrap } from './FocusTrap';
import { Button } from './Button';
import { Input } from './Input';
import { Box, Stack } from './Layout';

export default {
  title: 'Primitives / FocusTrap',
};

export const Active = () => {
  const [open, setOpen] = useState(false);
  return (
    <Box className="max-w-md p-8">
      <Stack gap={3}>
        {!open ? (
          <Button onClick={() => setOpen(true)}>Open trap</Button>
        ) : (
          <FocusTrap className="mc-glass space-y-3 rounded-lg p-4">
            <p className="text-sm text-foreground">
              Tab cycles between the inputs and the close button. Focus cannot escape this card.
            </p>
            <Input placeholder="Trapped input 1" />
            <Input placeholder="Trapped input 2" />
            <Button variant="outline" onClick={() => setOpen(false)}>
              Close
            </Button>
          </FocusTrap>
        )}
        <p className="text-xs text-muted-foreground">
          Outside content (this paragraph) cannot receive focus while the trap is active.
        </p>
      </Stack>
    </Box>
  );
};

export const Inactive = () => (
  <Box className="max-w-md p-8">
    <FocusTrap active={false} className="mc-glass rounded-lg p-4">
      <p className="text-sm text-foreground">
        With <code>active=false</code> the trap is a no-op; focus flows freely through the surrounding page.
      </p>
    </FocusTrap>
  </Box>
);
