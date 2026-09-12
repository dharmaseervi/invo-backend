"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { ApiError, auth } from "@/lib/api";
import { AuthShell } from "@/components/AuthShell";
import { Button, ErrorText, Field } from "@/components/ui";

export default function SignupPage() {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  function submit(e: React.FormEvent) {
    e.preventDefault();
    if (password.length < 6) {
      setError("Password must be at least 6 characters.");
      return;
    }

    setBusy(true);
    setError("");
    void (async () => {
      try {
        const res = await auth.register(email.trim(), password);
        // The server creates the account even when the code cannot be sent, so the
        // next screen is the same either way — it just explains which happened.
        const q = new URLSearchParams({
          email: res.email,
          sent: res.email_sent ? "1" : "0",
        });
        router.replace(`/verify?${q.toString()}`);
      } catch (err) {
        setError(err instanceof ApiError ? err.message : "Something went wrong.");
      } finally {
        setBusy(false);
      }
    })();
  }

  return (
    <AuthShell title="Create your account" subtitle="Start invoicing in minutes">
      <ErrorText>{error}</ErrorText>
      <form onSubmit={submit}>
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
          autoComplete="new-password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          hint="At least 6 characters"
          required
        />
        <Button type="submit" variant="primary" loading={busy} className="w-full">
          Create account
        </Button>
      </form>

      <p className="mt-5 text-center text-label-12 text-muted">
        By creating an account you agree to our{" "}
        <a href="/terms" className="text-accent hover:underline">
          Terms of Service
        </a>{" "}
        and{" "}
        <a href="/privacy" className="text-accent hover:underline">
          Privacy Policy
        </a>
        .
      </p>

      <p className="mt-5 text-center text-sm text-muted">
        Already have an account?{" "}
        <Link href="/login" className="text-accent hover:underline">
          Sign in
        </Link>
      </p>
    </AuthShell>
  );
}
