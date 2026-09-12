"use client";

import { Suspense, useCallback, useEffect, useMemo, useState } from "react";
import { useSearchParams } from "next/navigation";
import {
  ApiError,
  clients as clientsApi,
  formatDate,
  formatMoney,
  ledger as ledgerApi,
  type Client,
  type LedgerEntry,
} from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { downloadCsv } from "@/lib/csv";
import { AppShell, NoCompany } from "@/components/AppShell";
import {
  Badge,
  Button,
  Card,
  CardHead,
  EmptyState,
  SearchInput,
  Select,
  SkeletonRows,
  Stat,
  TableWrap,
  Td,
  Th,
} from "@/components/ui";

/** What each ledger row came from, in words a shopkeeper uses. */
const SOURCE_LABEL: Record<string, string> = {
  INVOICE: "Invoice",
  PAYMENT: "Payment",
  CREDIT_NOTE: "Credit note",
  OPENING: "Opening balance",
};

export default function LedgerPage() {
  return (
    // useSearchParams suspends during prerender, so the boundary is required.
    <Suspense fallback={<AppShell title="Ledger">{null}</AppShell>}>
      <Ledger />
    </Suspense>
  );
}

function Ledger() {
  const params = useSearchParams();
  const { company, loading: authLoading } = useAuth();

  const [clientList, setClientList] = useState<Client[]>([]);
  // Pre-filtered when arrived at from a client's page, so the link lands on that
  // account rather than on everything.
  const [clientId, setClientId] = useState(params.get("client") ?? "");
  const [entries, setEntries] = useState<LedgerEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [search, setSearch] = useState("");

  const companyId = company?.id ?? null;

  useEffect(() => {
    if (!companyId) return;
    let cancelled = false;
    void (async () => {
      try {
        const res = await clientsApi.list(companyId);
        if (!cancelled) setClientList(res.clients ?? []);
      } catch {
        /* the picker stays empty; the company ledger still loads */
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [companyId]);

  const load = useCallback(async () => {
    if (!companyId) return;
    setLoading(true);
    setError("");
    try {
      // No client selected means the whole company — every customer's movements in
      // one run, which is what you want when reconciling a day's takings.
      const res = clientId
        ? await ledgerApi.forClient(Number(clientId), companyId)
        : await ledgerApi.forCompany(companyId);
      setEntries(res.data ?? []);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to load the ledger");
    } finally {
      setLoading(false);
    }
  }, [companyId, clientId]);

  useEffect(() => {
    if (!companyId) return;
    void (async () => {
      await load();
    })();
  }, [companyId, load]);

  const rows = useMemo(() => {
    const term = search.trim().toLowerCase();
    if (!term) return entries;
    return entries.filter((e) =>
      `${e.client_name} ${e.description} ${e.source_type}`.toLowerCase().includes(term),
    );
  }, [entries, search]);

  // Debit is what the customer owes, credit is what they have settled. The closing
  // balance is the last row's running balance, not a sum of the column.
  const totals = useMemo(() => {
    const debit = entries.reduce((s, e) => s + (e.debit ?? 0), 0);
    const credit = entries.reduce((s, e) => s + (e.credit ?? 0), 0);
    return { debit, credit, balance: debit - credit };
  }, [entries]);

  if (authLoading || !company) {
    return <AppShell title="Ledger">{authLoading ? null : <NoCompany />}</AppShell>;
  }

  const selectedClient = clientList.find((c) => String(c.id) === clientId);

  return (
    <AppShell
      title="Ledger"
      description={
        selectedClient
          ? `Every movement on ${selectedClient.name}'s account`
          : "Every invoice, payment and credit note, in order"
      }
      actions={
        <Button
          icon="download"
          disabled={rows.length === 0}
          onClick={() =>
            downloadCsv(
              `ledger${selectedClient ? `-${selectedClient.name}` : ""}.csv`,
              ["Date", "Client", "Type", "Description", "Debit", "Credit", "Balance"],
              rows.map((e) => [
                e.created_at,
                e.client_name,
                SOURCE_LABEL[e.source_type] ?? e.source_type,
                e.description,
                e.debit || "",
                e.credit || "",
                e.balance,
              ]),
            )
          }
        >
          Export
        </Button>
      }
    >
      <div className="mb-5 grid gap-3 sm:grid-cols-3">
        <Stat label="Billed" value={formatMoney(totals.debit)} icon="invoice" />
        <Stat label="Received" value={formatMoney(totals.credit)} icon="payment" />
        <Stat
          label="Outstanding"
          value={formatMoney(totals.balance)}
          tone={totals.balance > 0 ? "warning" : undefined}
          sub={totals.balance <= 0 ? "Nothing owed" : undefined}
        />
      </div>

      <div className="mb-4 flex flex-wrap items-end gap-3">
        <div className="w-full sm:w-56">
          <Select
            label="Account"
            value={clientId}
            onChange={(e) => setClientId(e.target.value)}
            className="!mb-0"
          >
            <option value="">All clients</option>
            {clientList.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </Select>
        </div>
        <SearchInput
          value={search}
          onChange={setSearch}
          placeholder="Search description or client"
        />
      </div>

      <Card>
        <CardHead
          title="Movements"
          action={
            <span className="text-xs text-muted">
              {rows.length} {rows.length === 1 ? "entry" : "entries"}
            </span>
          }
        />
        {loading && entries.length === 0 ? (
          <SkeletonRows />
        ) : error ? (
          <EmptyState
            icon="alert"
            title="Couldn't load the ledger"
            message={error}
            action={<Button onClick={() => void load()}>Try again</Button>}
          />
        ) : rows.length === 0 ? (
          <EmptyState
            icon="ledger"
            title={search ? "No matches" : "Nothing recorded yet"}
            message={
              search
                ? `Nothing matched "${search}".`
                : "Issue an invoice or record a payment and it will appear here."
            }
          />
        ) : (
          <div className="p-2">
            <TableWrap min="44rem">
              <thead>
                <tr>
                  <Th>Date</Th>
                  {!clientId && <Th>Client</Th>}
                  <Th>Entry</Th>
                  <Th align="right">Debit</Th>
                  <Th align="right">Credit</Th>
                  <Th align="right">Balance</Th>
                </tr>
              </thead>
              <tbody>
                {rows.map((e) => (
                  <tr key={e.id} className="transition hover:bg-subtle/60">
                    <Td className="whitespace-nowrap text-muted">
                      {formatDate(e.created_at)}
                    </Td>
                    {!clientId && <Td className="font-medium">{e.client_name}</Td>}
                    <Td>
                      <div className="flex items-center gap-2">
                        <Badge tone={e.credit > 0 ? "success" : "neutral"}>
                          {SOURCE_LABEL[e.source_type] ?? e.source_type}
                        </Badge>
                        <span className="truncate text-muted">{e.description}</span>
                      </div>
                    </Td>
                    <Td align="right" className="tabular">
                      {e.debit ? formatMoney(e.debit) : "—"}
                    </Td>
                    <Td align="right" className="tabular text-success">
                      {e.credit ? formatMoney(e.credit) : "—"}
                    </Td>
                    <Td align="right" className="tabular font-medium">
                      {formatMoney(e.balance)}
                    </Td>
                  </tr>
                ))}
              </tbody>
            </TableWrap>
          </div>
        )}
      </Card>
    </AppShell>
  );
}
