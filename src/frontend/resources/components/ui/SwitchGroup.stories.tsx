// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { useState } from 'react';
import { SwitchGroup } from './SwitchGroup';
import { Box, Stack } from './Layout';

const notificationOptions = [
  {
    value: 'container-start',
    label: 'Container started',
    description: 'Whenever a container transitions to running.',
  },
  {
    value: 'container-stop',
    label: 'Container stopped',
    description: 'Whenever a container exits cleanly.',
  },
  {
    value: 'image-pull',
    label: 'Image pull failures',
    description: 'When a `docker pull` fails for any reason.',
  },
  {
    value: 'agent-offline',
    label: 'Agent offline',
    description: 'Remote agent stops heartbeating for over 60 s.',
  },
];

export default {
  title: 'Primitives / SwitchGroup',
};

export const Vertical = () => {
  const [values, setValues] = useState<string[]>(['container-start']);
  return (
    <Box className="max-w-md p-8">
      <SwitchGroup
        label="Notifications"
        description="Pick which events should notify you in-app and over webhooks."
        values={values}
        onChange={setValues}
        options={notificationOptions}
      />
    </Box>
  );
};

export const Horizontal = () => {
  const [values, setValues] = useState<string[]>(['info', 'warning']);
  return (
    <Box className="max-w-2xl p-8">
      <SwitchGroup
        label="Severity"
        values={values}
        onChange={setValues}
        orientation="horizontal"
        options={[
          { value: 'info', label: 'Info' },
          { value: 'success', label: 'Success' },
          { value: 'warning', label: 'Warning' },
          { value: 'error', label: 'Error' },
          { value: 'critical', label: 'Critical' },
        ]}
      />
    </Box>
  );
};

export const WithDisabled = () => (
  <Box className="max-w-md p-8">
    <SwitchGroup
      label="Disabled options"
      values={['available']}
      onChange={() => undefined}
      options={[
        { value: 'available', label: 'Available option' },
        { value: 'blocked', label: 'Blocked by plan', disabled: true },
      ]}
    />
  </Box>
);

export const AllDisabled = () => (
  <Box className="max-w-md p-8">
    <Stack gap={3}>
      <SwitchGroup
        label="Locked"
        description="All options are disabled while the section is read-only."
        disabled
        values={[]}
        onChange={() => undefined}
        options={notificationOptions}
      />
    </Stack>
  </Box>
);
