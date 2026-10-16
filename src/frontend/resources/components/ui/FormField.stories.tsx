// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { FormField } from './FormField';
import { Input } from './Input';
import { Textarea } from './Textarea';
import { Select } from './Select';
import { Box, Stack } from './Layout';

export default {
  title: 'Primitives / FormField',
};

export const Basic = () => (
  <Box className="max-w-sm p-8">
    <FormField label="Container name" htmlFor="ff-name">
      <Input id="ff-name" placeholder="nginx-proxy" />
    </FormField>
  </Box>
);

export const WithDescription = () => (
  <Box className="max-w-sm p-8">
    <FormField label="Restart policy" description="What Docker should do when the container exits.">
      <Input placeholder="unless-stopped" />
    </FormField>
  </Box>
);

export const Stacked = () => (
  <Box className="max-w-md p-8">
    <Stack gap={4}>
      <FormField label="Image">
        <Input placeholder="nginx:1.27" />
      </FormField>
      <FormField label="Hostname">
        <Input placeholder="web-01" />
      </FormField>
      <FormField label="Restart policy" description="Defaults to `no` if left blank.">
        <Select
          value="unless-stopped"
          onChange={() => undefined}
          options={[
            { value: 'no', label: 'no' },
            { value: 'always', label: 'always' },
            { value: 'unless-stopped', label: 'unless-stopped' },
            { value: 'on-failure', label: 'on-failure' },
          ]}
        />
      </FormField>
      <FormField label="Notes">
        <Textarea placeholder="Anything else the next operator should know." />
      </FormField>
    </Stack>
  </Box>
);
