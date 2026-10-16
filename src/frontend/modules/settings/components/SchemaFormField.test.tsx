// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import { beforeAll, describe, expect, it } from 'vitest';
import { SchemaFormField } from './SchemaFormField';
import type { ConfigFieldDecl } from '../hooks/useCommunicationAdapters';
import { i18n } from '../../../test/i18n';
import { renderWithProviders } from '../../../test/render';

const field: ConfigFieldDecl = {
  key: 'host',
  type: 'string',
  label: 'SMTP host',
  label_key: 'config.host.label',
  help: 'English help',
  help_key: 'config.host.help',
};

describe('SchemaFormField plugin keys', () => {
  beforeAll(() => {
    i18n.addResourceBundle(
      'en',
      'example-plugin',
      { config: { host: { label: 'Translated host', help: 'Translated help' } } },
      true,
      true,
    );
  });

  it('renders label and help from the owning plugin namespace when the keys resolve', () => {
    renderWithProviders(<SchemaFormField field={field} value="" onChange={() => {}} i18nNs="example-plugin" />);
    expect(screen.getByText('Translated host')).toBeInTheDocument();
    expect(screen.getByText('Translated help')).toBeInTheDocument();
    expect(screen.queryByText('SMTP host')).not.toBeInTheDocument();
  });

  it('falls back to the English label and help when a key does not resolve', () => {
    const missing = { ...field, label_key: 'config.nope.label', help_key: 'config.nope.help' };
    renderWithProviders(<SchemaFormField field={missing} value="" onChange={() => {}} i18nNs="example-plugin" />);
    expect(screen.getByText('SMTP host')).toBeInTheDocument();
    expect(screen.getByText('English help')).toBeInTheDocument();
  });

  it('uses the English text when no namespace is supplied', () => {
    renderWithProviders(<SchemaFormField field={field} value="" onChange={() => {}} />);
    expect(screen.getByText('SMTP host')).toBeInTheDocument();
  });
});
