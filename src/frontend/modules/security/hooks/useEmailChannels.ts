// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';

/**
 * Enabled communication channels that can actually carry an email.
 *
 * "Can carry an email" is not a list of channel types kept here — it is
 * `supportsEmail` on the adapter, which the backend derives from the
 * plugin's own declared config schema (a sender plus a recipient list).
 * A Slack channel and an SMTP channel are both rows in the same table;
 * only the second one has somewhere to put a From address, and only the
 * backend can see which is which.
 *
 * Two requests rather than one because the two facts live apart: the
 * channels an operator configured, and what their adapters can do.
 */

type CommunicationChannelRow = {
  id: string;
  name: string;
  channelType: string;
  isDefault: boolean;
  enabled: boolean;
};

type AdapterCapabilityRow = {
  channelType: string;
  displayName: string;
  supportsEmail: boolean;
};

export type EmailChannel = {
  id: string;
  name: string;
  channelType: string;
  /** The adapter's own label, e.g. "SMTP email". Falls back to the raw type. */
  typeLabel: string;
  isDefault: boolean;
};

export function useEmailChannels() {
  return useQuery({
    queryKey: ['communication-channels', 'email'],
    queryFn: async (): Promise<EmailChannel[]> => {
      const [channelsRes, adaptersRes] = await Promise.all([
        api.get<CommunicationChannelRow[]>('/communication-channels'),
        api.get<AdapterCapabilityRow[]>('/communication-channels/adapters'),
      ]);

      const emailTypes = new Map<string, string>();
      for (const adapter of adaptersRes.data ?? []) {
        if (adapter.supportsEmail) {
          emailTypes.set(adapter.channelType, adapter.displayName);
        }
      }

      return (channelsRes.data ?? [])
        .filter((channel) => channel.enabled && emailTypes.has(channel.channelType))
        .map((channel) => ({
          id: channel.id,
          name: channel.name,
          channelType: channel.channelType,
          typeLabel: emailTypes.get(channel.channelType) || channel.channelType,
          isDefault: channel.isDefault,
        }));
    },
    staleTime: 30_000,
  });
}
