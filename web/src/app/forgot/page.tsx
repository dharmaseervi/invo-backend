"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { ApiError, auth } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { AuthShell } from "@/components/AuthShell";
import { Button, ErrorText, Field, useToast } from "@/components/ui";

export default function ForgotPasswordPage() {
  const router = useRouter();
  const toast = useToast();
  const { signIn } = useAuth();

  const [stage, setStage] = useState<"request" | "reset">("request");
  const [email, setEmail] = useState("");
  const [code, setCode] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");

    void (async () => {
      try {
        if (stage === "request") {
          await auth.forgotPassword(email.trim());
          setStage("reset");
          toast.show("If that address has an account, a code is on its way.");
          return;
        }
        if (password.length < 6) {
          setError("Password must be at least 6 characters.");
          return;
        }
        // Reset signs the user straight in, and the server invalidates every token
        // issued before this moment — so any session an attacker still held is ended.
        const res = await auth.resetPassword(email.trim(), code.trim(), password);
        await signIn(res.token, res.user);
        toast.show("Password updated.", "success");
        router.replace("/dashboard");
      } catch (err) {
        setError(err instanceof ApiError ? err.message : "Something went wrong.");
      } finally {
        setBusy(false);
      }
    })();
  }

  return (
    <AuthShell
      title="Reset your password"
      subtitle={
        stage === "request"
          ? "We'll email you a 6-digit code"
          : `Enter the code sent to ${email}`
      }
    >
      <ErrorText>{error}</ErrorText>
      <form onSubmit={submit}>
        <Field
          label="Email"
          type="email"
          autoComplete="email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          required
          disabled={stage === "reset"}
        />
        {stage === "reset" && (
          <>
            <Field
              label="6-digit code"
              inputMode="numeric"
              autoComplete="one-time-code"
              maxLength={6}
              value={code}
              onChange={(e) => setCode(e.target.value)}
              required
            />
            <Field
              label="New password"
              type="password"
              autoComplete="new-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              hint="At least 6 characters"
              required
            />
          </>
        )}
        <Button type="submit" variant="primary" loading={busy} className="w-full">
          {stage === "request" ? "Send code" : "Reset password"}
        </Button>
      </form>

      <p className="mt-6 text-center text-sm text-muted">
        <Link href="/login" className="text-accent hover:underline">
          Back to sign in
        </Link>
      </p>
    </AuthShell>
  );
}
