"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "@/lib/auth";
import { Spinner } from "@/components/ui";

// Entry point: a static export cannot redirect on the server, so the decision is
// made here on first paint.
export default function Home() {
  const router = useRouter();
  // Waits for the provider to have actually tried the session. Deciding from a token
  // in localStorage sent a browser holding a perfectly good session cookie to the
  // login screen, because in production there is no token there to find.
  const { loading, signedIn } = useAuth();

  useEffect(() => {
    if (loading) return;
    router.replace(signedIn ? "/dashboard" : "/login");
  }, [loading, signedIn, router]);

  return (
    <div className="grid h-full place-items-center">
      <Spinner />
    </div>
  );
}
