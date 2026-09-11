"use client";

import { Suspense, useState } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { ApiError, auth } from "@/lib/api";
import { AuthShell } from "@/components/AuthShell";
import { Button, ErrorText, Field, Spinner, useToast } from "@/components/ui";

function VerifyForm() {
  const router = useRouter();
  const params = useSearchParams();
  const toast = useToast();

  const email = params.get("email") ?? "";
  // Signup reports whether the code actually went out, so the page can say "check
  // your email" or "we couldn't send it" rather than guessing.
  const [sent, setSent] = useState(params.get("sent") !== "0");
  const [code, setCode] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    void (async () => {
      try {
        await auth.verifyEmail(email, code.trim());
        toast.show("Email verified — you can sign in now.", "success");
        router.replace("/login");
      } catch (err) {
        setError(err instanceof ApiError ? err.message : "Something went wrong.");
      } finally {
        setBusy(false);
      }
    })();
  }

  function resend() {
    setBusy(true);
    setError("");
    void (async () => {
      try {
        await auth.resendVerification(email);
        setSent(true);
        toast.show("Code sent.", "success");
      } catch (err) {
        setError(err instanceof ApiError ? err.message : "Couldn't send the code.");
      } finally {
        setBusy(false);
      }
    })();
  }

  return (
    <AuthShell
      title="Verify your email"
      subtitle={
        sent
          ? `We sent a 6-digit code to ${email}`
          : "We couldn't send the code. Tap resend to try again."
      }
    >
      <ErrorText>{error}</ErrorText>
      <form onSubmit={submit}>
        <Field
          label="6-digit code"
          inputMode="numeric"
          autoComplete="one-time-code"
          maxLength={6}
          value={code}
          onChange={(e) => setCode(e.target.value)}
          required
        />
        <Button type="submit" variant="primary" loading={busy} className="w-full">
          Verify
        </Button>
      </form>

      <Button onClick={resend} disabled={busy} className="mt-3 w-full">
        Resend code
      </Button>

      <p className="mt-6 text-center text-sm text-muted">
        <Link href="/login" className="text-accent hover:underline">
          Back to sign in
        </Link>
      </p>
    </AuthShell>
  );
}

export default function VerifyPage() {
  // useSearchParams needs a Suspense boundary when the page is statically exported.
  return (
    <Suspense
      fallback={
        <div className="grid h-full place-items-center">
          <Spinner />
        </div>
      }
    >
      <VerifyForm />
    </Suspense>
  );
}
