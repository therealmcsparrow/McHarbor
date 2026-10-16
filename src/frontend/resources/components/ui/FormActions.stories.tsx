// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { IconAlertTriangle } from '@tabler/icons-react';
import { FormActions } from './FormActions';
import { Box } from './Layout';

export default {
  title: 'Primitives / FormActions',
};

export const Default = () => (
  <Box className="max-w-2xl border-border bg-card p-8">
    <FormActions onSave={() => undefined} onCancel={() => undefined} canSave />
  </Box>
);

export const Saving = () => (
  <Box className="max-w-2xl border-border bg-card p-8">
    <FormActions onSave={() => undefined} onCancel={() => undefined} isSaving />
  </Box>
);

export const WithWarning = () => (
  <Box className="max-w-2xl border-border bg-card p-8">
    <FormActions
      onSave={() => undefined}
      onCancel={() => undefined}
      warning={
        <span className="flex items-center gap-1.5 text-warning">
          <IconAlertTriangle className="size-3.5" />
          Saving recreates the container.
        </span>
      }
    />
  </Box>
);

export const DestructiveSave = () => (
  <Box className="max-w-2xl border-border bg-card p-8">
    <FormActions onSave={() => undefined} saveVariant="destructive" saveLabel="Wipe environment" hideCancel />
  </Box>
);

export const Compact = () => (
  <Box className="max-w-2xl border-border bg-card p-8">
    <FormActions onSave={() => undefined} onCancel={() => undefined} variant="compact" saveLabel="Apply filter" />
  </Box>
);
