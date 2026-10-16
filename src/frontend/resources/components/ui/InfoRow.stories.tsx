// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { Badge } from './Badge';
import { InfoRow } from './InfoRow';
import { Box } from './Layout';

export default {
  title: 'Primitives / InfoRow',
};

export const Basic = () => (
  <Box className="max-w-md p-8">
    <InfoRow label="Container ID">3a1b…f9c2</InfoRow>
    <InfoRow label="Image">nginx:1.27</InfoRow>
    <InfoRow label="Created">3 days ago</InfoRow>
    <InfoRow label="Status">
      <Badge variant="success">running</Badge>
    </InfoRow>
  </Box>
);

export const MonoValue = () => (
  <Box className="max-w-md p-8">
    <InfoRow label="Socket">unix:///var/run/docker.sock</InfoRow>
    <InfoRow label="Compose path">/srv/mcharbor/stacks/portainer.yml</InfoRow>
  </Box>
);
