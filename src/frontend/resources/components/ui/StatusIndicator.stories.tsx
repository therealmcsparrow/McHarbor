// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { StatusIndicator } from './StatusIndicator';
import { Box, Flex, Stack } from './Layout';
import { Caption } from './Typography';
import type { StatusTone } from '@resources/theme/status-palette';

export default {
  title: 'Primitives / StatusIndicator',
};

const TONES: StatusTone[] = ['success', 'warning', 'danger', 'critical', 'info', 'neutral'];

/**
 * All six tones, all three variants. Every value here resolves through the
 * theme token contract, so switching theme in the Ladle toolbar restyles
 * this page — which is the property the whole exercise was for.
 */
export const AllTones = () => (
  <Box className="p-8">
    <Stack gap={6}>
      <Stack gap={2}>
        <Caption>badge</Caption>
        <Flex wrap gap={2}>
          {TONES.map((tone) => (
            <StatusIndicator key={tone} tone={tone} label={tone} variant="badge" />
          ))}
        </Flex>
      </Stack>

      <Stack gap={2}>
        <Caption>pill (outline)</Caption>
        <Flex wrap gap={2}>
          {TONES.map((tone) => (
            <StatusIndicator key={tone} tone={tone} label={tone} variant="pill" />
          ))}
        </Flex>
      </Stack>

      <Stack gap={2}>
        <Caption>dot</Caption>
        <Flex wrap gap={3}>
          {TONES.map((tone) => (
            <StatusIndicator key={tone} tone={tone} variant="dot" ariaLabel={tone} />
          ))}
        </Flex>
      </Stack>
    </Stack>
  </Box>
);

export const Sizes = () => (
  <Box className="p-8">
    <Flex wrap gap={3} align="center">
      <StatusIndicator tone="success" label="running" size="sm" />
      <StatusIndicator tone="success" label="running" size="md" />
    </Flex>
  </Box>
);

/**
 * Only the running state pulses — it is the one that is continuously true
 * rather than a fact about the past. Turning motion down in Settings, or
 * setting `prefers-reduced-motion`, stops it.
 */
export const Pulse = () => (
  <Box className="p-8">
    <Flex wrap gap={4} align="center">
      <StatusIndicator tone="success" label="running" pulse />
      <StatusIndicator tone="neutral" label="exited" />
    </Flex>
  </Box>
);
