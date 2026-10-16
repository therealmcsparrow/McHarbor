// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { StatusBadge, CONTAINER_STATUS, STACK_STATUS, ENVIRONMENT_STATUS, RECONCILER_STATUS } from './StatusBadge';
import { Box, Flex, Stack } from './Layout';
import { Caption } from './Typography';

export default {
  title: 'Primitives / StatusBadge',
};

export const Containers = () => (
  <Box className="p-8">
    <Flex wrap gap={2}>
      {Object.keys(CONTAINER_STATUS).map((status) => (
        <StatusBadge key={status} status={status} map={CONTAINER_STATUS} />
      ))}
    </Flex>
  </Box>
);

export const Stacks = () => (
  <Box className="p-8">
    <Flex wrap gap={2}>
      {Object.keys(STACK_STATUS).map((status) => (
        <StatusBadge key={status} status={status} map={STACK_STATUS} />
      ))}
    </Flex>
  </Box>
);

export const Environments = () => (
  <Box className="p-8">
    <Flex wrap gap={2}>
      {Object.keys(ENVIRONMENT_STATUS).map((status) => (
        <StatusBadge key={status} status={status} map={ENVIRONMENT_STATUS} />
      ))}
    </Flex>
  </Box>
);

export const Reconciler = () => (
  <Box className="p-8">
    <Flex wrap gap={2}>
      {Object.keys(RECONCILER_STATUS).map((status) => (
        <StatusBadge key={status} status={status} map={RECONCILER_STATUS} />
      ))}
    </Flex>
  </Box>
);

export const UnknownFallsBack = () => (
  <Stack gap={2} className="p-8">
    <Caption tone="muted">
      When the status isn't in the supplied map the badge renders a muted `secondary` variant.
    </Caption>
    <StatusBadge status="weird-new-state" map={CONTAINER_STATUS} />
  </Stack>
);
