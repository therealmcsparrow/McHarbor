// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { AgentDockerImage, AgentTokenDialog, copyAgentText } from './AgentTokenDialog';

const toastMock = vi.hoisted(() => ({
  error: vi.fn(),
  success: vi.fn(),
}));

vi.mock('sonner', () => ({
  toast: toastMock,
}));

function setClipboard(value: unknown) {
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value });
}

function renderDialog() {
  render(<AgentTokenDialog open onOpenChange={vi.fn()} token="agt_test" serverUrl="https://mcharbor.example.test" />);
}

async function openDockerTab(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('tab', { name: 'Docker run' }));
}

describe('AgentTokenDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    Object.defineProperty(window, 'isSecureContext', { configurable: true, value: true });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('loads the agent image from McHarbor in the Docker install command', async () => {
    const user = userEvent.setup();
    renderDialog();
    await openDockerTab(user);

    const dockerCommand = screen.getByText((content) => content.includes('docker run -d')).textContent ?? '';

    expect(dockerCommand).toContain(AgentDockerImage);
    expect(dockerCommand).toContain('Authorization: Bearer agt_test');
    expect(dockerCommand).toContain('https://mcharbor.example.test/api/agent/image/linux/');
    expect(dockerCommand).not.toContain('ghcr.io');
    expect(dockerCommand).not.toContain('docker pull');
    expect(dockerCommand).toContain('MCHARBOR_URL=https://mcharbor.example.test');
    expect(dockerCommand).toContain('MCHARBOR_AGENT_TOKEN=agt_test');
    expect(dockerCommand).not.toMatch(/\s+mcharbor-agent\s*$/);
  });

  it('copies the generated token from the dialog', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    const user = userEvent.setup();
    setClipboard({ writeText });
    renderDialog();

    await user.click(screen.getAllByRole('button', { name: 'Copy token' })[0]!);

    await waitFor(() => expect(writeText).toHaveBeenCalledWith('agt_test'));
    expect(toastMock.success).toHaveBeenCalledWith('Copied to clipboard');
  });

  it('copies the published Docker command from the dialog', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    const user = userEvent.setup();
    setClipboard({ writeText });
    renderDialog();
    await openDockerTab(user);

    await user.click(screen.getByRole('button', { name: 'Copy Docker command' }));

    await waitFor(() => {
      const copiedCommand = writeText.mock.calls[0]?.[0] as string | undefined;
      expect(copiedCommand).toContain(AgentDockerImage);
      expect(copiedCommand).not.toMatch(/\s+mcharbor-agent\s*$/);
    });
    expect(toastMock.success).toHaveBeenCalledWith('Copied to clipboard');
  });

  it('reports the clipboard API as unsupported when it is missing', async () => {
    setClipboard(undefined);

    await expect(copyAgentText('fallback text')).resolves.toEqual({ ok: false, reason: 'unsupported' });
  });

  it('opens the manual copy dialog when the clipboard refuses', async () => {
    const user = userEvent.setup();
    setClipboard({ writeText: vi.fn().mockRejectedValue(new Error('denied')) });
    renderDialog();

    await user.click(screen.getAllByRole('button', { name: 'Copy token' })[0]!);

    expect(await screen.findByText((content) => content.startsWith('Copy manually'))).toBeInTheDocument();
    expect(toastMock.success).not.toHaveBeenCalled();
  });
});
