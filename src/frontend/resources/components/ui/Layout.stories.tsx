// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { Box, Flex, Grid, Stack } from './Layout';

export default {
  title: 'Primitives / Layout',
};

export const FlexShowcase = () => (
  <div className="space-y-4 p-8">
    <Flex align="center" justify="between" gap={3} className="rounded-md border border-border p-4">
      <span>left</span>
      <span>center</span>
      <span>right</span>
    </Flex>
    <Flex direction="col" gap={2} className="rounded-md border border-border p-4">
      <span>row 1</span>
      <span>row 2</span>
      <span>row 3</span>
    </Flex>
    <Flex wrap gap={2} className="rounded-md border border-border p-4">
      <span className="rounded bg-muted px-3 py-1">wrap-1</span>
      <span className="rounded bg-muted px-3 py-1">wrap-2</span>
      <span className="rounded bg-muted px-3 py-1">wrap-3</span>
    </Flex>
  </div>
);

export const GridShowcase = () => (
  <div className="p-8">
    <Grid columns={3} gap={3} className="grid-cols-1 sm:grid-cols-2 md:grid-cols-3">
      {[1, 2, 3, 4, 5, 6].map((n) => (
        <Box key={n} className="mc-glass rounded-md p-4 text-center">
          cell {n}
        </Box>
      ))}
    </Grid>
  </div>
);

export const StackShowcase = () => (
  <div className="p-8">
    <Stack gap={3}>
      <Box className="mc-glass rounded-md p-4">item 1</Box>
      <Box className="mc-glass rounded-md p-4">item 2</Box>
      <Box className="mc-glass rounded-md p-4">item 3</Box>
    </Stack>
  </div>
);
