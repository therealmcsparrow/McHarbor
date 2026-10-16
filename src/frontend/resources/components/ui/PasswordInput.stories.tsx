// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { PasswordInput } from './PasswordInput';
import { Box, Stack } from './Layout';

export default {
  title: 'Primitives / PasswordInput',
};

export const Basic = () => (
  <Box className="max-w-sm p-8">
    <PasswordInput placeholder="Enter agent token" />
  </Box>
);

export const Sizes = () => (
  <Stack gap={4} className="max-w-sm p-8">
    <PasswordInput size="sm" placeholder="Small" />
    <PasswordInput size="md" placeholder="Medium (default)" />
    <PasswordInput size="lg" placeholder="Large" />
  </Stack>
);

export const VisibleByDefault = () => (
  <Box className="max-w-sm p-8">
    <PasswordInput defaultVisible placeholder="Visible on load" />
  </Box>
);

export const Disabled = () => (
  <Box className="max-w-sm p-8">
    <PasswordInput disabled value="redacted" />
  </Box>
);
