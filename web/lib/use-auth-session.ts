"use client";

import { useCallback, useEffect, useState } from "react";
import type { AuthSession } from "@/lib/api";

const storageKey = "shaka-auth-session";

function parseSession(value: string | null): AuthSession | null {
  if (!value) return null;
  const parsed: unknown = JSON.parse(value);
  if (
    typeof parsed !== "object" ||
    parsed === null ||
    !("accessToken" in parsed) ||
    typeof parsed.accessToken !== "string" ||
    !("expiresAt" in parsed) ||
    typeof parsed.expiresAt !== "string" ||
    !("email" in parsed) ||
    typeof parsed.email !== "string" ||
    Date.parse(parsed.expiresAt) <= Date.now()
  ) {
    return null;
  }
  return {
    accessToken: parsed.accessToken,
    expiresAt: parsed.expiresAt,
    email: parsed.email,
  };
}

export function useAuthSession() {
  const [session, setSession] = useState<AuthSession | null>(null);
  const [ready, setReady] = useState(false);
  const [notice, setNotice] = useState("");

  useEffect(() => {
    let active = true;
    Promise.resolve().then(() => {
      if (!active) return;
      try {
        const saved = window.sessionStorage.getItem(storageKey);
        const restored = parseSession(saved);
        setSession(restored);
        if (saved && !restored) window.sessionStorage.removeItem(storageKey);
      } catch {
        setNotice("Could not restore your sign-in session in this browser.");
      } finally {
        setReady(true);
      }
    });
    return () => {
      active = false;
    };
  }, []);

  useEffect(() => {
    if (!session) return;
    const timeout = window.setTimeout(() => {
      setSession(null);
      setNotice("Your session expired. Sign in to continue.");
      try {
        window.sessionStorage.removeItem(storageKey);
      } catch {
        setNotice("Your session expired, but this browser could not clear its saved token.");
      }
    }, Math.max(0, Date.parse(session.expiresAt) - Date.now()));
    return () => window.clearTimeout(timeout);
  }, [session]);

  const signIn = useCallback((nextSession: AuthSession) => {
    setSession(nextSession);
    setNotice("");
    try {
      window.sessionStorage.setItem(storageKey, JSON.stringify(nextSession));
    } catch {
      setNotice("Signed in, but this browser could not save your session.");
    }
  }, []);

  const signOut = useCallback(() => {
    setSession(null);
    setNotice("");
    try {
      window.sessionStorage.removeItem(storageKey);
    } catch {
      setNotice("Could not clear the saved session from this browser.");
    }
  }, []);

  return { session, ready, notice, signIn, signOut, setNotice };
}
