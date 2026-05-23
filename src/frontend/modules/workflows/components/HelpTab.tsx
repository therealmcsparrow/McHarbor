// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { HelpTabGeneralHelp } from './HelpTabGeneralHelp';
import { HelpTabNodeDocumentation } from './HelpTabNodeDocumentation';
import type { NodeDefinition } from '../types';

type HelpTabProps = {
  definition?: NodeDefinition;
};

export function HelpTab({ definition }: HelpTabProps) {
  if (definition) {
    return <HelpTabNodeDocumentation definition={definition} />;
  }
  return <HelpTabGeneralHelp />;
}
