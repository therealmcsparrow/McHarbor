// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ParamType, TargetKind } from './runbooks';

export type RunbookParamSpec = {
  name: string;
  type: ParamType;
  target?: TargetKind;
  options?: string[];
  min?: number;
  max?: number;
  required: boolean;
};

export type RunbookCatalogEntry = {
  key: string;
  description: string;
  /** A plugin entry names its wording by key in the plugin's own namespace instead of carrying it. */
  descriptionKey?: string;
  descriptionNs?: string;
  params?: string[];
  paramSpecs?: RunbookParamSpec[];
  requiredPermissions: string[];
  destructive: boolean;
  streams: boolean;
  plugin?: string;
  target?: TargetKind;
};

/** A plugin selector kind the server can list, titled by a key in the plugin's own namespace. */
export type RunbookTargetKind = {
  kind: TargetKind;
  labelKey: string;
  labelNs: string;
  /** What an unpinned selector means, in `labelNs`, when it is not the run's own subject. */
  emptyLabelKey?: string;
};

/** An attach kind a loaded plugin declares: titled in the plugin's namespace, its subject picked from the `selector` target kind. */
export type RunbookAttachKind = {
  kind: string;
  labelKey: string;
  labelNs: string;
  selector: TargetKind;
};

export type RunbookCatalog = {
  actions: RunbookCatalogEntry[];
  diagnostics: RunbookCatalogEntry[];
  targets?: RunbookTargetKind[];
  attachKinds?: RunbookAttachKind[];
};

export type RunbookTargetOption = {
  id: string;
  name: string;
  detail?: string;
};
