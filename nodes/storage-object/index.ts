// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const storageObject: NodeDefinition = {
  key: 'storage-object',
  label: 'Storage Object',
  category: 'action',
  description: 'List, upload, download or delete objects on a storage location through its adapter',
  icon: 'IconCloudUpload',
  configSchema: [
    { key: 'storage_location_id', label: 'Storage Location', type: 'storage-location-select', required: true },
    {
      key: 'action',
      label: 'Action',
      type: 'select',
      required: true,
      default: 'list',
      options: [
        { value: 'list', label: 'List' },
        { value: 'upload', label: 'Upload' },
        { value: 'download', label: 'Download' },
        { value: 'delete', label: 'Delete' },
      ],
    },
    { key: 'prefix', label: 'Prefix', type: 'text', required: false, showWhen: { action: 'list' } },
    {
      key: 'key',
      label: 'Object Key',
      type: 'text',
      required: false,
      showWhen: { action: ['upload', 'download', 'delete'] },
    },
    {
      key: 'source_mode',
      label: 'Source',
      type: 'select',
      required: false,
      default: 'property',
      options: [
        { value: 'property', label: 'Message property' },
        { value: 'text', label: 'Inline text' },
      ],
      showWhen: { action: 'upload' },
    },
    {
      key: 'property',
      label: 'Source Property',
      type: 'expression',
      required: false,
      default: 'payload',
      showWhen: { action: 'upload', source_mode: 'property' },
    },
    {
      key: 'content_encoding',
      label: 'Source Encoding',
      type: 'select',
      required: false,
      default: 'auto',
      options: [
        { value: 'auto', label: 'Auto (text or JSON)' },
        { value: 'base64', label: 'Base64' },
      ],
      showWhen: { action: 'upload', source_mode: 'property' },
    },
    {
      key: 'content',
      label: 'Content',
      type: 'textarea',
      required: false,
      showWhen: { action: 'upload', source_mode: 'text' },
    },
    {
      key: 'output_encoding',
      label: 'Output Encoding',
      type: 'select',
      required: false,
      default: 'text',
      options: [
        { value: 'text', label: 'Text' },
        { value: 'json', label: 'JSON' },
        { value: 'base64', label: 'Base64' },
      ],
      showWhen: { action: 'download' },
    },
    {
      key: 'output_property',
      label: 'Output Property',
      type: 'expression',
      required: false,
      default: 'payload',
      showWhen: { action: 'download' },
    },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
