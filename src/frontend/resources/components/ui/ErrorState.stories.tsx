// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none (ErrorState reads `common.errorState.*`).

import { ErrorState } from './ErrorState';
import { Box, Stack } from './Layout';

export default {
  title: 'Primitives / ErrorState',
};

export const Banner = () => (
  <Box className="max-w-2xl p-8">
    <ErrorState
      title="Couldn't reach the Docker daemon"
      description="Make sure /var/run/docker.sock is mounted and the daemon is running."
      onRetry={() => undefined}
      errorId="err_3f9a2b"
    />
  </Box>
);

export const Card = () => (
  <Box className="max-w-md p-8">
    <ErrorState
      variant="card"
      title="Image pull failed"
      description="We couldn't pull `nginx:1.27` from the configured registry."
      onRetry={() => undefined}
    />
  </Box>
);

export const Page = () => (
  <Box className="p-8">
    <ErrorState
      variant="page"
      title="This page failed to load"
      description="An unexpected error occurred while rendering the dashboard."
      primaryAction={{ label: 'Reload page', onClick: () => undefined }}
      secondaryAction={{ label: 'Open log', onClick: () => undefined }}
      errorId="err_5d8c01"
    />
  </Box>
);

export const Inline = () => (
  <Box className="max-w-2xl p-8">
    <Stack gap={3}>
      <ErrorState
        variant="inline"
        title="Network driver missing"
        description="Selected network `macvlan-net` no longer exists."
        onRetry={() => undefined}
      />
      <ErrorState variant="inline" title="Disk quota exceeded" />
    </Stack>
  </Box>
);
