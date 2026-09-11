"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { ApiError, auth } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { AuthShell } from "@/components/AuthShell";
import { Button, ErrorText, Field } from "@/components/ui";

type Mode = "password" | "otp";

export default function LoginPage() {
  const router = useRouter();
  const { signIn } = useAuth();

  const [mode, setMode] = useState<Mode>("password");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const [otpSent, setOtpSent] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function run(fn: () => Promise<void>) {
    setBusy(true);
    setError("");
    try {
      await fn();
    } catch (err) {
      setError(
        err instanceof ApiError ? err.message : "Something went wrong.",
      );
    } finally {
      setBusy(false);
    }
  }

  const submitPassword = (e: React.FormEvent) => {
    e.preventDefault();
    void run(async () => {
      const res = await auth.login(email.trim(), password);
      await signIn(res.token, res.user);
      router.replace("/dashboard");
    });
  };

  const submitOtp = (e: React.FormEvent) => {
    e.preventDefault();
    void run(async () => {
      if (!otpSent) {
        await auth.sendOtp(email.trim());
        setOtpSent(true);
        return;
      }
      const res = await auth.verifyOtp(email.trim(), code.trim());
      await signIn(res.token, res.user);
      router.replace("/dashboard");
    });
  };

  return (
    <AuthShell title="Sign in" subtitle="GST invoicing for Indian businesses">
      <ErrorText>{error}</ErrorText>

      {mode === "password" ? (
        <form onSubmit={submitPassword}>
          <Field
            label="Email"
            type="email"
            autoComplete="email"
            placeholder="name@company.com"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            required
          />
          <Field
            label="Password"
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
          />
          <Button type="submit" variant="primary" loading={busy} className="w-full">
            Sign in
          </Button>
        </form>
      ) : (
        <form onSubmit={submitOtp}>
          <Field
            label="Email"
            type="email"
            autoComplete="email"
            placeholder="name@company.com"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            required
            disabled={otpSent}
          />
          {otpSent && (
            <Field
              label="6-digit code"
              inputMode="numeric"
              autoComplete="one-time-code"
              maxLength={6}
              value={code}
              onChange={(e) => setCode(e.target.value)}
              hint={`Sent to ${email}`}
              required
            />
          )}
          <Button type="submit" variant="primary" loading={busy} className="w-full">
            {otpSent ? "Verify and sign in" : "Send code"}
          </Button>
          {otpSent && (
            <button
              type="button"
              onClick={() => {
                setOtpSent(false);
                setCode("");
              }}
              className="mt-3 w-full text-center text-sm text-muted hover:text-ink"
            >
              Use a different email
            </button>
          )}
        </form>
      )}

      <div className="my-5 flex items-center gap-3 text-xs text-muted">
        <span className="h-px flex-1 bg-line" />
        or
        <span className="h-px flex-1 bg-line" />
      </div>

      <Button
        onClick={() => {
          setMode(mode === "password" ? "otp" : "password");
          setError("");
          setOtpSent(false);
        }}
        className="w-full"
      >
        {mode === "password" ? "Sign in with a code instead" : "Use password instead"}
      </Button>

      <div className="mt-6 space-y-2 text-center text-sm text-muted">
        <p>
          <Link href="/forgot" className="text-accent hover:underline">
            Forgot password?
          </Link>
        </p>
        <p>
          No account?{" "}
          <Link href="/signup" className="text-accent hover:underline">
            Sign up
          </Link>
        </p>
      </div>
    </AuthShell>
  );
}
