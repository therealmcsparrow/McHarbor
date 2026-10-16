// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { waitForServiceWorker } from '@core/pwa/register';

export type PushDevice = {
  id: string;
  label: string;
  userAgent: string;
  lastSuccessAt?: string;
  failureCount: number;
  createdAt: string;
};

export type PushTestResult = { sent: number; failed: number; removed: number };

export type PushSupport = 'supported' | 'unsupported' | 'insecure' | 'ios-install';

const DEVICE_KEY = 'mcharbor-push-device-id';
const DEVICES = ['oncall', 'push', 'devices'] as const;
const LOCAL = ['oncall', 'push', 'local'] as const;

export function base64UrlToBytes(value: string): Uint8Array<ArrayBuffer> {
  const padded = `${value}${'='.repeat((4 - (value.length % 4)) % 4)}`.replace(/-/g, '+').replace(/_/g, '/');
  const raw = atob(padded);
  const bytes = new Uint8Array(new ArrayBuffer(raw.length));
  for (let i = 0; i < raw.length; i++) bytes[i] = raw.charCodeAt(i);
  return bytes;
}

export function detectPushSupport(): PushSupport {
  const ios = /iPad|iPhone|iPod/.test(navigator.userAgent);
  const standalone =
    (typeof window.matchMedia === 'function' && window.matchMedia('(display-mode: standalone)').matches) ||
    (navigator as Navigator & { standalone?: boolean }).standalone === true;
  if (ios && !standalone) return 'ios-install';
  if (!window.isSecureContext) return 'insecure';
  if (!('serviceWorker' in navigator) || !('PushManager' in window) || !('Notification' in window))
    return 'unsupported';
  return 'supported';
}

function readDeviceId(): string | null {
  try {
    return localStorage.getItem(DEVICE_KEY);
  } catch {
    return null;
  }
}

function writeDeviceId(id: string | null): void {
  try {
    if (id) localStorage.setItem(DEVICE_KEY, id);
    else localStorage.removeItem(DEVICE_KEY);
  } catch {
    return;
  }
}

async function localSubscription(): Promise<PushSubscription | null> {
  if (detectPushSupport() !== 'supported') return null;
  const registration = await navigator.serviceWorker.getRegistration('/');
  return (await registration?.pushManager.getSubscription()) ?? null;
}

export function usePushDevices() {
  return useQuery({
    queryKey: DEVICES,
    queryFn: () => api.get<PushDevice[]>('/push/subscriptions').then(assertSuccess),
  });
}

export function useLocalPushSubscription() {
  return useQuery({ queryKey: LOCAL, queryFn: localSubscription, networkMode: 'always' });
}

function useInvalidatePush() {
  const queryClient = useQueryClient();
  return () => {
    void queryClient.invalidateQueries({ queryKey: DEVICES });
    void queryClient.invalidateQueries({ queryKey: LOCAL });
  };
}

export function useEnablePush() {
  const invalidate = useInvalidatePush();
  return useMutation({
    mutationFn: async (label: string) => {
      const permission = await Notification.requestPermission();
      if (permission !== 'granted') throw new Error('permission-denied');
      const key = await api.get<{ publicKey: string }>('/push/vapid-public-key').then(assertSuccess);
      const registration = await waitForServiceWorker();
      const subscription = await registration.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: base64UrlToBytes(key.publicKey),
      });
      const json = subscription.toJSON();
      const saved = await api.post<PushDevice>('/push/subscriptions', {
        endpoint: json.endpoint,
        keys: { p256dh: json.keys?.p256dh ?? '', auth: json.keys?.auth ?? '' },
        label,
        userAgent: navigator.userAgent,
      });
      if (!saved.success) {
        await subscription.unsubscribe();
        return assertSuccess(saved);
      }
      const device = assertSuccess(saved);
      writeDeviceId(device.id);
      return device;
    },
    onSettled: invalidate,
  });
}

export function useRevokePushDevice() {
  const invalidate = useInvalidatePush();
  const { t } = useTranslation('oncall');
  return useMutation({
    mutationFn: async (id: string) => {
      await api.del(`/push/subscriptions/${encodeURIComponent(id)}`).then(assertSuccess);
      if (id === readDeviceId()) {
        writeDeviceId(null);
        await (await localSubscription())?.unsubscribe();
      }
    },
    meta: { success: t('device.revoked') },
    onSettled: invalidate,
  });
}

export function useDisablePush() {
  const invalidate = useInvalidatePush();
  return useMutation({
    mutationFn: async () => {
      const id = readDeviceId();
      if (id) {
        const res = await api.del(`/push/subscriptions/${encodeURIComponent(id)}`);
        if (!res.success && res.code !== 'err.not_found') assertSuccess(res);
      }
      writeDeviceId(null);
      await (await localSubscription())?.unsubscribe();
    },
    onSettled: invalidate,
  });
}

export function useSendTestPush() {
  return useMutation({
    mutationFn: () => api.post<PushTestResult>('/push/test').then(assertSuccess),
  });
}
