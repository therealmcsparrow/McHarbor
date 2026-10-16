// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { renderWithProviders } from '../../../test/render';
import { BackupKeyInstallDialog } from './BackupKeyInstallDialog';

describe('BackupKeyInstallDialog', () => {
  it('on a native install, describes the data-directory file and offers no Compose project path', () => {
    renderWithProviders(<BackupKeyInstallDialog open native onOpenChange={() => {}} />);
    expect(screen.getByText(/writes it to its data directory and restarts/)).toBeInTheDocument();
    expect(screen.queryByText('Advanced')).not.toBeInTheDocument();
    expect(screen.queryByText(/mcharbor_backup_key secret/)).not.toBeInTheDocument();
  });

  it('in a container, keeps the Docker secret wording and the Compose project path option', () => {
    renderWithProviders(<BackupKeyInstallDialog open onOpenChange={() => {}} />);
    expect(screen.getByText(/installs it as the mcharbor_backup_key secret/)).toBeInTheDocument();
    expect(screen.getByText('Advanced')).toBeInTheDocument();
  });
});
