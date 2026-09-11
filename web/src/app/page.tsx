"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { getToken } from "@/lib/api";
import { Spinner } from "@/components/ui";

// Entry point: a static export cannot redirect on the server, so the decision is
// made here on first paint.
export default function Home() {
  const router = useRouter();

  useEffect(() => {
    router.replace(getToken() ? "/dashboard" : "/login");
  }, [router]);

  return (
    <div className="grid h-full place-items-center">
      <Spinner />
    </div>
  );
}
