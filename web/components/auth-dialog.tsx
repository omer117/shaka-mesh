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
  const [problems, setProblems] = useState<string[]>([]);

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setLoading(true);
    setError("");
    setProblems([]);

    const formData = new FormData(event.currentTarget);
    const email = String(formData.get("email") ?? "").trim().toLowerCase();
    const password = String(formData.get("password") ?? "");

    try {
      if (mode === "register") await registerUser(email, password);
      onAuthenticated(await loginUser(email, password));
    } catch (caught) {
      if (caught instanceof ApiError && caught.status === 429) {
        const wait = caught.retryAfterSeconds;
        setError(
          wait
            ? `Too many attempts. Please wait ${wait} second${wait === 1 ? "" : "s"} and try again.`
            : "Too many attempts. Please wait a moment and try again.",
        );
      } else if (caught instanceof ApiError) {
        setError(caught.problems.length > 0 ? "Please choose a stronger password:" : caught.message);
        setProblems(caught.problems);
      } else {
        setError("Could not reach the Shaka API. Please try again.");
      }
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
              minLength={mode === "register" ? 10 : undefined}
              maxLength={72}
              aria-describedby={mode === "register" ? "password-hint" : undefined}
              required
            />
          </label>
          {mode === "register" && (
            <p id="password-hint" className="dialog-copy">
              At least 10 characters. Avoid common passwords and your email address.
            </p>
          )}
          {error && (
            <div className="dialog-error" role="alert">
              <p>{error}</p>
              {problems.length > 0 && (
                <ul>
                  {problems.map((problem) => (
                    <li key={problem}>Password {problem}</li>
                  ))}
                </ul>
              )}
            </div>
          )}
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
              setProblems([]);
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
