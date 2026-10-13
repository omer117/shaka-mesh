"use client";

import { FormEvent, useState } from "react";
import { X } from "lucide-react";
import { ApiError, loginUser, registerUser, type AuthSession } from "@/lib/api";

interface AuthDialogProps {
  onClose: () => void;
  onAuthenticated: (session: AuthSession) => void;
}

export default function AuthDialog({
  onClose,
  onAuthenticated,
}: AuthDialogProps) {
  const [mode, setMode] = useState<"login" | "register">("login");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setLoading(true);
    setError("");

    const formData = new FormData(event.currentTarget);
    const email = String(formData.get("email") ?? "").trim().toLowerCase();
    const password = String(formData.get("password") ?? "");

    try {
      if (mode === "register") await registerUser(email, password);
      onAuthenticated(await loginUser(email, password));
    } catch (caught) {
      setError(
        caught instanceof ApiError
          ? caught.message
          : "Could not reach the Shaka API. Please try again.",
      );
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="dialog-backdrop" onMouseDown={onClose}>
      <section
        className="dialog-card auth-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="auth-dialog-title"
        onMouseDown={(event) => event.stopPropagation()}
      >
        <button className="dialog-close" aria-label="Close" onClick={onClose}>
          <X size={18} />
        </button>
        <div className="dialog-eyebrow">JOIN THE LINEUP</div>
        <h2 id="auth-dialog-title">
          {mode === "login" ? "Welcome back." : "Find your people."}
        </h2>
        <p className="dialog-copy">
          {mode === "login"
            ? "Sign in to share local spots and conditions."
            : "Create an account to contribute to the Shaka community."}
        </p>
        <form className="dialog-form" onSubmit={submit}>
          <label>
            Email
            <input
              name="email"
              type="email"
              autoComplete="email"
              maxLength={320}
              required
            />
          </label>
          <label>
            Password
            <input
              name="password"
              type="password"
              autoComplete={mode === "login" ? "current-password" : "new-password"}
              minLength={8}
              maxLength={72}
              required
            />
          </label>
          {error && <p className="dialog-error" role="alert">{error}</p>}
          <button className="dialog-submit" type="submit" disabled={loading}>
            {loading
              ? "One moment…"
              : mode === "login"
                ? "Sign in"
                : "Create account"}
          </button>
        </form>
        <p className="dialog-switch">
          {mode === "login" ? "New to Shaka?" : "Already have an account?"}{" "}
          <button
            type="button"
            onClick={() => {
              setError("");
              setMode(mode === "login" ? "register" : "login");
            }}
          >
            {mode === "login" ? "Create account" : "Sign in"}
          </button>
        </p>
      </section>
    </div>
  );
}
