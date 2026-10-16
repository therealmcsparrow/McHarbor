// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { useState } from 'react';
import { FileUpload } from './FileUpload';
import { Box } from './Layout';

export default {
  title: 'Primitives / FileUpload',
};

export const Basic = () => {
  const [, setFiles] = useState<File[]>([]);
  return (
    <Box className="max-w-lg p-8">
      <FileUpload label="Compose file" description="Drop a docker-compose.yml or import a stack." onChange={setFiles} />
    </Box>
  );
};

export const Multiple = () => {
  const [, setFiles] = useState<File[]>([]);
  return (
    <Box className="max-w-lg p-8">
      <FileUpload
        label="TLS bundle"
        description="Upload the cert chain and key together."
        multiple
        accept=".crt,.pem,.key"
        maxFiles={5}
        onChange={setFiles}
      />
    </Box>
  );
};

export const SizeLimited = () => {
  const [, setFiles] = useState<File[]>([]);
  return (
    <Box className="max-w-lg p-8">
      <FileUpload
        label="Icon"
        description="Max 1 MB, PNG / SVG only."
        accept=".png,.svg,image/*"
        maxSize={1024 * 1024}
        onChange={setFiles}
      />
    </Box>
  );
};

export const Disabled = () => (
  <Box className="max-w-lg p-8">
    <FileUpload label="Locked" disabled onChange={() => undefined} />
  </Box>
);
