"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { getToken } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { Button, Card, Spinner } from "@/components/ui";

export default function DashboardPage() {
  const router = useRouter();
  const { loading, company, companies, signOut } = useAuth();

  // Route guard. The server rejects an unauthenticated request regardless — this
  // only avoids rendering a signed-in shell to someone who is not.
  useEffect(() => {
    if (!loading && !getToken()) router.replace("/login");
  }, [loading, router]);

  if (loading) {
    return (
      <div className="grid h-full place-items-center">
        <Spinner />
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-5xl p-8">
      <div className="mb-6 flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Dashboard</h1>
          <p className="mt-1 text-sm text-muted">
            {company ? company.name : "No company yet"}
          </p>
        </div>
        <Button onClick={() => void signOut()}>Sign out</Button>
      </div>

      <Card className="p-6">
        <p className="text-sm text-muted">
          Signed in successfully. {companies.length} compan
          {companies.length === 1 ? "y" : "ies"} on this account.
        </p>
      </Card>
    </div>
  );
}
