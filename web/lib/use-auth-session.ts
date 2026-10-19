"use client";

import { useCallback, useEffect, useState } from "react";
import { logoutUser, refreshSession, type AuthSession } from "@/lib/api";

// The access token lives in React state only (never in web storage). The
// long-lived refresh token is an HttpOnly cookie that JavaScript cannot read;
// this hook just asks the API for a fresh access token when needed.

// Refresh this long before the access token expires.
const refreshLeadMs = 30_000;
const retryDelayMs = 10_000;

// One refresh at a time per tab. Refresh tokens rotate, so two overlapping
// requests with the same cookie would look like token reuse to the server and
// revoke the session. React StrictMode and multiple hook users both trigger this.
let inFlight: Promise<AuthSession | null> | null = null;

function refreshOnce(): Promise<AuthSession | null> {
  if (inFlight) return inFlight;
  const run = () => refreshSession();
  // navigator.locks serializes refreshes across tabs too: a waiting tab sends
  // the rotated cookie the first tab just received.
  const locked: Promise<AuthSession | null> =
    typeof navigator !== "undefined" && navigator.locks
      ? // The lock callback's promise is awaited by request(); Promise.resolve
        // flattens the (over-nested) DOM typing.
        Promise.resolve(navigator.locks.request("shaka-auth-refresh", run))
      : run();
  inFlight = locked.finally(() => {
    inFlight = null;
  });
  return inFlight;
}

export function useAuthSession() {
  const [session, setSession] = useState<AuthSession | null>(null);
  const [ready, setReady] = useState(false);
  const [notice, setNotice] = useState("");
  const applySession = setSession;

  // Restore the session on page load from the refresh cookie.
  useEffect(() => {
    let active = true;
    refreshOnce()
      .then((restored) => {
        if (active && restored) applySession(restored);
      })
      .catch(() => {
        if (active) {
          setNotice("Could not reach the server to restore your sign-in session.");
        }
      })
      .finally(() => {
        if (active) setReady(true);
      });
    return () => {
      active = false;
    };
  }, [applySession]);

  // Silently renew shortly before the access token expires.
  useEffect(() => {
    if (!session) return;
    let active = true;
    let timeout: number | undefined;

    const renew = async () => {
      try {
        const next = await refreshOnce();
        if (!active) return;
        if (next) {
          // Keep the known email if the server omitted it.
          applySession({ ...next, email: next.email || session.email });
        } else {
          applySession(null);
          setNotice("Your session expired. Sign in to continue.");
        }
      } catch {
        // Transient failure (offline, 5xx): stay signed in and retry.
        if (active) timeout = window.setTimeout(renew, retryDelayMs);
      }
    };

    const delay = Date.parse(session.expiresAt) - Date.now() - refreshLeadMs;
    timeout = window.setTimeout(renew, Math.max(0, delay));

    // Background tabs throttle timers; re-check when the tab becomes visible.
    const onVisible = () => {
      if (
        document.visibilityState === "visible" &&
        Date.parse(session.expiresAt) - Date.now() < refreshLeadMs
      ) {
        window.clearTimeout(timeout);
        void renew();
      }
    };
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      active = false;
      window.clearTimeout(timeout);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, [session, applySession]);

  const signIn = useCallback(
    (nextSession: AuthSession) => {
      applySession(nextSession);
      setNotice("");
    },
    [applySession],
  );

  const signOut = useCallback(() => {
    applySession(null);
    setNotice("");
    // Revoke the refresh-token family server-side and clear the cookie.
    logoutUser().catch(() => {
      setNotice("Signed out here, but the server could not be reached to end the session.");
    });
  }, [applySession]);

  return { session, ready, notice, signIn, signOut, setNotice };
}
