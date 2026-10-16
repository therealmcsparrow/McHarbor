// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { SeverityIcon } from './SeverityIcon';
import { Box, Flex, Stack } from './Layout';

export default {
  title: 'Primitives / SeverityIcon',
};

export const AllLevels = () => (
  <Flex align="center" gap={4} className="p-8">
    <SeverityIcon severity="info" />
    <SeverityIcon severity="success" />
    <SeverityIcon severity="warning" />
    <SeverityIcon severity="error" />
    <SeverityIcon severity="critical" />
  </Flex>
);

export const Sizes = () => (
  <Flex align="center" gap={4} className="p-8">
    <SeverityIcon severity="warning" size="sm" />
    <SeverityIcon severity="warning" size="md" />
    <SeverityIcon severity="warning" size="lg" />
  </Flex>
);

export const WithLabel = () => (
  <Stack gap={2} className="p-8">
    <SeverityIcon severity="info" ariaLabel="Info" />
    <SeverityIcon severity="success" ariaLabel="Success" />
    <SeverityIcon severity="warning" ariaLabel="Warning" />
    <SeverityIcon severity="error" ariaLabel="Error" />
    <SeverityIcon severity="critical" ariaLabel="Critical" />
  </Stack>
);

export const UnknownFallsBack = () => (
  <Box className="p-8">
    <SeverityIcon severity="some-future-level" ariaLabel="Unknown" />
  </Box>
);
