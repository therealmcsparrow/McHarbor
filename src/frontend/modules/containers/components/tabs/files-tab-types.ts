// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { FileEntry } from '@core/types/docker';

export type DialogState =
  | { type: 'none' }
  | { type: 'view'; entry: FileEntry }
  | { type: 'edit'; entry: FileEntry }
  | { type: 'create'; mode: 'file' | 'folder' }
  | { type: 'rename'; entry: FileEntry }
  | { type: 'chmod'; entry: FileEntry }
  | { type: 'upload' }
  | { type: 'delete'; entry: FileEntry };
