"use client";

import Link from "next/link";
import { Suspense, useCallback, useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import {
  ApiError,
  clientAddresses,
  clients as clientsApi,
  formatDate,
  formatMoney,
  ledger as ledgerApi,
  type Client,
  type ClientAddress,
  type ClientInvoiceRow,
  type LedgerEntry,
} from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { AppShell, NoCompany } from "@/components/AppShell";
import { Icon } from "@/components/icons";
import {
  Badge,
  Button,
  Card,
  CardHead,
  EmptyState,
  Skeleton,
  SkeletonRows,
  Stat,
  TableWrap,
  Td,
  Th,
} from "@/components/ui";

/**
 * One client's whole relationship on a page: what they owe, what they have been
 * invoiced, and every movement on their account.
 *
 * Addressed by query string rather than a /clients/[id] route because the app is a
 * static export — a dynamic segment would have to be pre-rendered at build time, and
 * the ids are not known then.
 */
export default function ClientDetailPage() {
  return (
    // useSearchParams suspends during prerender, so the boundary is required.
    <Suspense fallback={<AppShell title="Client">{null}</AppShell>}>
      <ClientDetail />
    </Suspense>
  );
}

function statusTone(status: string, overdue: boolean) {
  if (status === "paid") return "success" as const;
  if (status === "cancelled" || status === "draft") return "neutral" as const;
  if (overdue) return "danger" as const;
  if (status === "partial") return "warning" as const;
  return "info" as const;
}

function ClientDetail() {
  const params = useSearchParams();
  const id = Number(params.get("id"));
  const { company, loading: authLoading } = useAuth();

  const [client, setClient] = useState<Client | null>(null);
  const [address, setAddress] = useState<ClientAddress | null>(null);
  const [invoices, setInvoices] = useState<ClientInvoiceRow[]>([]);
  const [entries, setEntries] = useState<LedgerEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const companyId = company?.id ?? null;

  const load = useCallback(async () => {
    if (!companyId || !id) return;
    setLoading(true);
    setError("");
    try {
      // There is no single-client endpoint, so the client comes from the company's
      // list. Everything else is fetched together rather than in sequence.
      const [list, invoiceRes] = await Promise.all([
        clientsApi.list(companyId),
        clientsApi.invoices(id),
      ]);
      const found = (list.clients ?? []).find((c) => c.id === id) ?? null;
      if (!found) {
        setError("That client is not on this company.");
        return;
      }
      setClient(found);
      setInvoices(invoiceRes.data ?? []);

      // These two are allowed to fail without taking the page down: the address is
      // optional, and the ledger is supporting detail rather than the subject.
      const [addr, led] = await Promise.allSettled([
        clientAddresses.get(id, "billing"),
        ledgerApi.forClient(id, companyId),
      ]);
      if (addr.status === "fulfilled") setAddress(addr.value.data);
      if (led.status === "fulfilled") setEntries(led.value.data ?? []);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to load this client");
    } finally {
      setLoading(false);
    }
  }, [companyId, id]);

  useEffect(() => {
    if (!companyId) return;
    void (async () => {
      await load();
    })();
  }, [companyId, load]);

  if (authLoading || !company) {
    return <AppShell title="Client">{authLoading ? null : <NoCompany />}</AppShell>;
  }

  if (!id) {
    return (
      <AppShell title="Client">
        <Card>
          <EmptyState
            icon="client"
            title="No client selected"
            message="Open a client from the list to see their account."
            action={
              <Link href="/clients">
                <Button variant="primary">Back to clients</Button>
              </Link>
            }
          />
        </Card>
      </AppShell>
    );
  }

  if (error) {
    return (
      <AppShell title="Client">
        <Card>
          <EmptyState
            icon="alert"
            title="Couldn't load this client"
            message={error}
            action={<Button onClick={() => void load()}>Try again</Button>}
          />
        </Card>
      </AppShell>
    );
  }

  // Outstanding comes from the invoices themselves rather than the ledger, so a
  // cancelled invoice cannot leave a balance behind.
  const open = invoices.filter((i) => i.status !== "cancelled" && i.status !== "draft");
  const billed = open.reduce((s, i) => s + (i.total ?? 0), 0);
  const paid = open.reduce((s, i) => s + (i.paid_amount ?? 0), 0);
  const outstanding = open.reduce((s, i) => s + (i.remaining_amount ?? 0), 0);
  const overdueCount = invoices.filter(
    (i) =>
      i.remaining_amount > 0 &&
      i.status !== "cancelled" &&
      i.status !== "draft" &&
      new Date(i.due_date) < new Date(),
  ).length;

  return (
    <AppShell
      title={client?.name ?? "Client"}
      description={
        client
          ? [client.phone, client.email].filter(Boolean).join(" · ") || "No contact details"
          : undefined
      }
      actions={
        <>
          <Link href={`/ledger?client=${id}`}>
            <Button icon="ledger">Ledger</Button>
          </Link>
          {/* The sidebar already has Clients; on a narrow screen the client's own
              name matters more than a second way back to the list. */}
          <Link href="/clients" className="hidden sm:block">
            <Button>All clients</Button>
          </Link>
        </>
      }
    >
      <div className="mb-5 grid gap-3 sm:grid-cols-3">
        {loading && !client ? (
          Array.from({ length: 3 }).map((_, i) => (
            <Card key={i} className="p-4">
              <Skeleton className="h-3 w-20" />
              <Skeleton className="mt-3 h-6 w-28" />
            </Card>
          ))
        ) : (
          <>
            <Stat label="Billed" value={formatMoney(billed)} icon="invoice" />
            <Stat label="Received" value={formatMoney(paid)} icon="payment" />
            <Stat
              label="Outstanding"
              value={formatMoney(outstanding)}
              tone={outstanding > 0 ? "warning" : undefined}
              sub={
                overdueCount > 0 ? (
                  <span className="text-danger">
                    {overdueCount} invoice{overdueCount === 1 ? "" : "s"} past due
                  </span>
                ) : outstanding === 0 ? (
                  "Nothing owed"
                ) : undefined
              }
            />
          </>
        )}
      </div>

      <div className="grid gap-5 lg:grid-cols-[1fr_20rem]">
        <div className="min-w-0 space-y-5">
          <Card>
            <CardHead
              title="Invoices"
              action={
                <span className="text-xs text-muted">
                  {invoices.length} {invoices.length === 1 ? "invoice" : "invoices"}
                </span>
              }
            />
            {loading && invoices.length === 0 ? (
              <SkeletonRows rows={3} />
            ) : invoices.length === 0 ? (
              <EmptyState
                icon="invoice"
                title="No invoices yet"
                message="Nothing has been billed to this client."
                action={
                  <Link href="/invoices">
                    <Button variant="primary" icon="plus">
                      New invoice
                    </Button>
                  </Link>
                }
              />
            ) : (
              <div className="p-2">
                <TableWrap min="34rem">
                  <thead>
                    <tr>
                      <Th>Invoice</Th>
                      <Th>Status</Th>
                      <Th align="right">Total</Th>
                      <Th align="right">Due</Th>
                    </tr>
                  </thead>
                  <tbody>
                    {invoices.map((inv) => {
                      const overdue =
                        inv.remaining_amount > 0 &&
                        inv.status !== "cancelled" &&
                        inv.status !== "draft" &&
                        new Date(inv.due_date) < new Date();
                      return (
                        <tr key={inv.id} className="transition hover:bg-subtle/60">
                          <Td>
                            <p className="font-medium">{inv.invoice_number || "Draft"}</p>
                            <p className="text-xs text-muted">
                              {formatDate(inv.invoice_date)}
                            </p>
                          </Td>
                          <Td>
                            <Badge tone={statusTone(inv.status, overdue)}>
                              {overdue
                                ? "Overdue"
                                : inv.status.charAt(0).toUpperCase() + inv.status.slice(1)}
                            </Badge>
                          </Td>
                          <Td align="right" className="tabular">
                            {formatMoney(inv.total)}
                          </Td>
                          <Td align="right" className="tabular">
                            {inv.remaining_amount > 0 ? (
                              formatMoney(inv.remaining_amount)
                            ) : (
                              <span className="text-muted">—</span>
                            )}
                          </Td>
                        </tr>
                      );
                    })}
                  </tbody>
                </TableWrap>
              </div>
            )}
          </Card>

          <Card>
            <CardHead
              title="Recent account activity"
              action={
                <Link
                  href={`/ledger?client=${id}`}
                  className="text-[13px] font-medium text-accent hover:underline"
                >
                  Full ledger
                </Link>
              }
            />
            {loading && entries.length === 0 ? (
              <SkeletonRows rows={3} />
            ) : entries.length === 0 ? (
              <EmptyState
                icon="ledger"
                title="Nothing recorded yet"
                message="Invoices, payments and credit notes appear here as they happen."
              />
            ) : (
              <ul className="divide-y divide-line">
                {/* Newest first, and only the last handful — the ledger page is for
                    reading the whole history. */}
                {[...entries]
                  .reverse()
                  .slice(0, 6)
                  .map((e) => (
                    <li key={e.id} className="flex items-center gap-3 px-5 py-3">
                      <span
                        className={`grid h-7 w-7 shrink-0 place-items-center rounded-full ${
                          e.credit > 0
                            ? "bg-success-soft text-success"
                            : "bg-subtle text-muted"
                        }`}
                      >
                        <Icon name={e.credit > 0 ? "payment" : "invoice"} className="h-3.5 w-3.5" />
                      </span>
                      <div className="min-w-0 flex-1">
                        <p className="truncate text-sm">{e.description}</p>
                        <p className="text-xs text-muted">{formatDate(e.created_at)}</p>
                      </div>
                      <span
                        className={`tabular text-sm font-medium ${
                          e.credit > 0 ? "text-success" : ""
                        }`}
                      >
                        {e.credit > 0
                          ? `− ${formatMoney(e.credit)}`
                          : formatMoney(e.debit)}
                      </span>
                    </li>
                  ))}
              </ul>
            )}
          </Card>
        </div>

        <div className="space-y-5">
          <Card>
            <CardHead title="Details" />
            <dl className="space-y-3 px-5 py-4 text-sm">
              <Detail label="Phone" value={client?.phone} />
              <Detail label="Email" value={client?.email} />
              <Detail
                label="Address"
                value={[client?.address, client?.city, client?.state, client?.pincode]
                  .filter(Boolean)
                  .join(", ")}
              />
              <Detail
                label="GSTIN"
                value={address?.gst_number ?? ""}
                mono
                empty="Unregistered"
              />
              <Detail
                label="Place of supply"
                value={client?.state}
                hint="Decides CGST/SGST or IGST on their invoices"
              />
            </dl>
          </Card>
        </div>
      </div>
    </AppShell>
  );
}

function Detail({
  label,
  value,
  hint,
  mono = false,
  empty = "—",
}: {
  label: string;
  value?: string | null;
  hint?: string;
  mono?: boolean;
  empty?: string;
}) {
  const shown = value?.trim();
  return (
    <div>
      <dt className="text-xs text-muted">{label}</dt>
      <dd className={`mt-0.5 ${shown ? "" : "text-muted"} ${mono ? "tabular" : ""}`}>
        {shown || empty}
      </dd>
      {hint && shown && <p className="mt-0.5 text-xs text-muted">{hint}</p>}
    </div>
  );
}
