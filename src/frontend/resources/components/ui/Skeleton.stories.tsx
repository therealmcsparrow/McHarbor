// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { Skeleton, CardSkeleton, TableSkeleton, ChartSkeleton, WidgetSkeleton } from './Skeleton';
import { Box, Flex, Grid } from './Layout';

export default {
  title: 'Primitives / Skeleton',
};

export const Basic = () => (
  <Box className="max-w-md space-y-3 p-8">
    <Skeleton className="h-5 w-2/3" />
    <Skeleton className="h-3 w-full" />
    <Skeleton className="h-3 w-5/6" />
    <Skeleton className="h-3 w-4/6" />
  </Box>
);

export const Rounded = () => (
  <Flex gap={4} className="p-8">
    <Skeleton rounded="sm" className="size-10" />
    <Skeleton rounded="md" className="size-10" />
    <Skeleton rounded="lg" className="size-10" />
    <Skeleton rounded="full" className="size-10" />
  </Flex>
);

export const CardSkeletonStory = () => (
  <Box className="max-w-md p-8">
    <CardSkeleton lines={4} />
  </Box>
);

export const TableSkeletonStory = () => (
  <Box className="max-w-2xl p-8">
    <TableSkeleton rows={4} columns={3} />
  </Box>
);

export const ChartSkeletonStory = () => (
  <Box className="max-w-lg p-8">
    <ChartSkeleton height={200} />
  </Box>
);

export const WidgetSkeletonStory = () => (
  <Grid columns={3} gap={4} className="grid-cols-1 p-8 sm:grid-cols-3">
    <WidgetSkeleton />
    <WidgetSkeleton />
    <WidgetSkeleton />
  </Grid>
);
