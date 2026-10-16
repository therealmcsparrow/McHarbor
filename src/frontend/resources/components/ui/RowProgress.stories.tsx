// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none (RowProgress reads `common.operations.*`).

import { RowProgress } from './RowProgress';
import { Box, Stack } from './Layout';

export default {
  title: 'Primitives / RowProgress',
};

export const Cell = () => (
  <Box className="max-w-md p-8">
    <RowProgress status="running" done={3} total={8} variant="cell" />
  </Box>
);

export const Inline = () => (
  <Box className="max-w-xl p-8">
    <RowProgress status="success" done={5} total={5} variant="inline" />
  </Box>
);

export const Compact = () => (
  <Box className="max-w-md p-8">
    <RowProgress status="running" done={1} total={3} variant="compact" />
  </Box>
);

export const AllStatuses = () => (
  <Stack gap={3} className="max-w-md p-8">
    <RowProgress status="idle" />
    <RowProgress status="running" done={2} total={5} />
    <RowProgress status="success" done={5} total={5} />
    <RowProgress status="error" done={1} total={5} />
    <RowProgress status="skipped" />
  </Stack>
);

export const Indeterminate = () => (
  <Box className="max-w-md p-8">
    <RowProgress status="running" />
  </Box>
);
