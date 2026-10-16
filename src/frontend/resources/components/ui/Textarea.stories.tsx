// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { Textarea } from './Textarea';
import { Box, Stack } from './Layout';

export default {
  title: 'Primitives / Textarea',
};

export const Basic = () => (
  <Box className="max-w-md p-8">
    <Textarea placeholder="Describe what this container does…" />
  </Box>
);

export const WithValue = () => (
  <Box className="max-w-md p-8">
    <Textarea
      defaultValue={`# compose fragment\nservices:\n  app:\n    image: nginx:1.27\n    ports:\n      - "8080:80"`}
    />
  </Box>
);

export const OutlineVariant = () => (
  <Box className="max-w-md p-8">
    <Textarea variant="outline" placeholder="Inline border style…" />
  </Box>
);

export const Disabled = () => (
  <Stack gap={4} className="max-w-md p-8">
    <Textarea disabled placeholder="Disabled textarea" />
    <Textarea disabled defaultValue="Read-only content" />
  </Stack>
);
