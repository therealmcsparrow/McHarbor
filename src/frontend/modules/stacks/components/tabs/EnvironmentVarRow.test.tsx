// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { ENV_REDACTION_MARKER } from '@resources/utils/env-redaction';
import { EnvironmentVarRow } from './EnvironmentVarRow';
import { grantPermissions } from '../../../../test/permissions';
import { renderWithProviders } from '../../../../test/render';

function renderRow(maskedValue: string) {
  const onToggle = vi.fn();
  renderWithProviders(
    <table>
      <tbody>
        <EnvironmentVarRow
          envKey="DB_PASSWORD"
          maskedValue={maskedValue}
          visible={false}
          onToggleVisibility={onToggle}
          onPromote={vi.fn()}
        />
      </tbody>
    </table>,
  );
  return { onToggle, button: screen.getByRole('button', { name: 'Show value' }) };
}

describe('EnvironmentVarRow reveal gate', () => {
  it('disables showing a masked value without stacks.manage and names the key', async () => {
    grantPermissions(['stacks.view']);
    const user = userEvent.setup();
    const { onToggle, button } = renderRow(ENV_REDACTION_MARKER);
    expect(button).toBeDisabled();
    await user.click(button);
    expect(onToggle).not.toHaveBeenCalled();
    await user.hover(button.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the stacks.manage permission')).not.toHaveLength(0);
  });

  it('lets stacks.manage reveal a masked value', async () => {
    grantPermissions(['stacks.view', 'stacks.manage']);
    const { onToggle, button } = renderRow(ENV_REDACTION_MARKER);
    await userEvent.click(button);
    expect(onToggle).toHaveBeenCalledWith('DB_PASSWORD');
  });

  it('needs no extra permission to show a value that was never masked', async () => {
    grantPermissions(['stacks.view']);
    const { onToggle, button } = renderRow('plain-value');
    await userEvent.click(button);
    expect(onToggle).toHaveBeenCalledWith('DB_PASSWORD');
  });
});
