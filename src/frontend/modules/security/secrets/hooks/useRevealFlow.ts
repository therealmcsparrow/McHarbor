// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState } from 'react';
import { useRevealSecret, useStepUp } from '@resources/hooks/useSecrets';
import { REVEAL_AUTO_HIDE_SECONDS, REVEAL_REASON_MIN_LENGTH, type Secret } from '../types';

/**
 * Owns the reveal state machine so the dialog stays presentational.
 *
 * The plaintext lives in this hook's state and nowhere else — not in the Query
 * cache (the underlying hook is a mutation for exactly that reason), not in a
 * store, not in localStorage. It is cleared when the dialog closes and
 * auto-hidden after REVEAL_AUTO_HIDE_SECONDS even if the dialog stays open,
 * because the realistic exposure is an unattended screen.
 */
export function useRevealFlow(isOpen: boolean, secret: Secret | null) {
  const reveal = useRevealSecret();
  const { reset: resetReveal } = reveal;
  const stepUp = useStepUp();

  const [reason, setReason] = useState('');
  const [value, setValue] = useState('');
  const [secondsLeft, setSecondsLeft] = useState(0);
  // The step-up proof. Held only until the submit that spends it — never
  // stored, never cached, cleared with the dialog.
  const [password, setPassword] = useState('');
  const [totpCode, setTotpCode] = useState('');
  const [steppedUp, setSteppedUp] = useState(false);

  useEffect(() => {
    if (isOpen) return;
    // eslint-disable-next-line react/set-state-in-effect -- closing the dialog must wipe the revealed secret and the entered credentials
    setReason('');
    setValue('');
    setSecondsLeft(0);
    setPassword('');
    setTotpCode('');
    setSteppedUp(false);
    resetReveal();
  }, [isOpen, resetReveal]);

  useEffect(() => {
    if (value === '') return;
    if (secondsLeft <= 0) {
      // eslint-disable-next-line react/set-state-in-effect -- the countdown reaching zero clears the revealed value
      setValue('');
      return;
    }
    const timer = window.setTimeout(() => setSecondsLeft((n) => n - 1), 1000);
    return () => window.clearTimeout(timer);
  }, [value, secondsLeft]);

  const reasonValid = reason.trim().length >= REVEAL_REASON_MIN_LENGTH;

  const revealNow = (secretId: string) => {
    reveal.mutate(
      { id: secretId, reason: reason.trim() },
      {
        onSuccess: (plaintext) => {
          setValue(plaintext);
          setSecondsLeft(REVEAL_AUTO_HIDE_SECONDS);
        },
      },
    );
  };

  const submit = () => {
    if (!secret || !reasonValid) return;
    if (!steppedUp) {
      // Mint the window, then reveal. One click for the user; two independent
      // checks on the server, which is the point.
      const secretId = secret.id;
      stepUp.mutate(
        { password: password || undefined, totpCode: totpCode || undefined },
        {
          onSuccess: () => {
            setSteppedUp(true);
            setPassword('');
            setTotpCode('');
            revealNow(secretId);
          },
        },
      );
      return;
    }
    revealNow(secret.id);
  };

  const submitDisabled =
    !reasonValid || reveal.isPending || stepUp.isPending || (!steppedUp && password === '' && totpCode === '');

  return {
    reason,
    setReason,
    value,
    secondsLeft,
    password,
    setPassword,
    totpCode,
    setTotpCode,
    steppedUp,
    reasonValid,
    submit,
    submitDisabled,
  };
}
