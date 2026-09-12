"use client";

import { useCallback, useEffect, useState } from "react";
import {
  ApiError,
  clients as clientsApi,
  formatDate,
  formatMoney,
  invoices as invoicesApi,
  payments as paymentsApi,
  today,
  type Client,
  type InvoiceRow,
  type PaymentRow,
} from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { AppShell, NoCompany } from "@/components/AppShell";
import {
  Button,
  Card,
  EmptyState,
  ErrorText,
  Field,
  Modal,
  Select,
  Spinner,
  useToast,
} from "@/components/ui";

const METHODS = ["Cash", "UPI", "Bank transfer", "Cheque", "Card", "Other"];

export default function PaymentsPage() {
  const { company, loading: authLoading } = useAuth();
  const toast = useToast();

  const [rows, setRows] = useState<PaymentRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [recording, setRecording] = useState(false);

  const companyId = company?.id ?? null;

  const load = useCallback(async () => {
    if (!companyId) return;
    setLoading(true);
    setError("");
    try {
      const res = await paymentsApi.list(companyId);
      setRows(res.payments ?? []);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to load payments");
    } finally {
      setLoading(false);
    }
  }, [companyId]);

  useEffect(() => {
    if (!companyId) return;
    void (async () => {
      await load();
    })();
  }, [companyId, load]);

  if (authLoading || !company) {
    return (
      <AppShell title="Payments">{authLoading ? <Spinner /> : <NoCompany />}</AppShell>
    );
  }

  return (
    <AppShell
      title="Payments"
      actions={
        <Button variant="primary" onClick={() => setRecording(true)}>
          Record payment
        </Button>
      }
    >
      <Card>
        {loading && rows.length === 0 ? (
          <div className="grid place-items-center py-16">
            <Spinner />
          </div>
        ) : error ? (
          <EmptyState
            title="Couldn't load payments"
            message={error}
            action={<Button onClick={() => void load()}>Try again</Button>}
          />
        ) : rows.length === 0 ? (
          <EmptyState
            title="No payments yet"
            message="Record what customers pay — it settles their oldest open invoices first."
            action={
              <Button variant="primary" onClick={() => setRecording(true)}>
                Record payment
              </Button>
            }
          />
        ) : (
          <ul className="divide-y divide-line">
            {rows.map((p) => (
              <li key={p.id} className="flex flex-wrap items-center gap-x-4 gap-y-2 px-5 py-4">
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{p.client_name}</p>
                  <p className="mt-0.5 truncate text-sm text-muted">
                    {[
                      formatDate(p.payment_date),
                      p.payment_method,
                      p.reference && `Ref ${p.reference}`,
                      // Which invoices a lump payment settled is otherwise impossible
                      // to work out from the amount alone.
                      p.applied_to && `Applied to ${p.applied_to}`,
                    ]
                      .filter(Boolean)
                      .join(" · ")}
                  </p>
                </div>
                <p className="tabular w-32 text-right text-sm font-medium text-success">
                  {formatMoney(p.amount)}
                </p>
              </li>
            ))}
          </ul>
        )}
      </Card>

      {recording && (
        <PaymentForm
          onClose={() => setRecording(false)}
          onSaved={() => {
            setRecording(false);
            toast.show("Payment recorded", "success");
            void load();
          }}
          companyId={company.id}
        />
      )}
    </AppShell>
  );
}

function PaymentForm({
  companyId,
  onClose,
  onSaved,
}: {
  companyId: number;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [clientList, setClientList] = useState<Client[]>([]);
  const [clientId, setClientId] = useState("");
  const [open, setOpen] = useState<InvoiceRow[]>([]);
  const [amount, setAmount] = useState("");
  const [method, setMethod] = useState(METHODS[0]);
  const [date, setDate] = useState(today());
  const [reference, setReference] = useState("");
  const [notes, setNotes] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const res = await clientsApi.list(companyId);
        if (!cancelled) setClientList(res.clients ?? []);
      } catch {
        /* the error surfaces on save */
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [companyId]);

  // Showing what is outstanding stops the commonest mistake: a payment larger than the
  // client's balance, which the server refuses outright.
  useEffect(() => {
    if (!clientId) {
      return;
    }
    let cancelled = false;
    void (async () => {
      try {
        const res = await invoicesApi.unpaid(Number(clientId), companyId);
        if (!cancelled) setOpen(res.data ?? []);
      } catch {
        if (!cancelled) setOpen([]);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [clientId, companyId]);

  const outstanding = open.reduce((sum, i) => sum + (i.remaining_amount ?? 0), 0);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    const value = Number(amount);
    if (!clientId) {
      setError("Pick a client");
      return;
    }
    if (!Number.isFinite(value) || value <= 0) {
      setError("Enter an amount greater than zero");
      return;
    }
    setSaving(true);
    setError("");
    try {
      await paymentsApi.record({
        client_id: Number(clientId),
        amount: value,
        payment_method: method,
        reference: reference.trim() || undefined,
        notes: notes.trim() || undefined,
        payment_date: date,
      });
      onSaved();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to record payment");
      setSaving(false);
    }
  };

  return (
    <Modal open title="Record payment" onClose={onClose}>
      <form onSubmit={submit} noValidate>
        <ErrorText>{error}</ErrorText>

        <Select label="Client" value={clientId} onChange={(e) => setClientId(e.target.value)}>
          <option value="">Select a client</option>
          {clientList.map((c) => (
            <option key={c.id} value={c.id}>
              {c.name}
            </option>
          ))}
        </Select>

        {clientId && (
          <p className="-mt-2 mb-4 text-xs text-muted">
            {open.length === 0
              ? "No open invoices for this client."
              : `${open.length} open invoice${open.length === 1 ? "" : "s"}, ${formatMoney(outstanding)} outstanding.`}
          </p>
        )}

        <div className="grid gap-x-4 sm:grid-cols-2">
          <Field
            label="Amount"
            type="number"
            step="0.01"
            min="0"
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
          />
          <Field
            label="Date received"
            type="date"
            value={date}
            onChange={(e) => setDate(e.target.value)}
            hint="Yesterday's cheque belongs to yesterday"
          />
        </div>

        <Select label="Method" value={method} onChange={(e) => setMethod(e.target.value)}>
          {METHODS.map((m) => (
            <option key={m} value={m}>
              {m}
            </option>
          ))}
        </Select>

        <div className="grid gap-x-4 sm:grid-cols-2">
          <Field
            label="Reference"
            value={reference}
            onChange={(e) => setReference(e.target.value)}
            placeholder="UTR, cheque no."
          />
          <Field label="Notes" value={notes} onChange={(e) => setNotes(e.target.value)} />
        </div>

        <p className="mb-2 text-xs text-muted">
          Applied to the oldest open invoices first.
        </p>

        <div className="mt-4 flex justify-end gap-2">
          <Button type="button" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" variant="primary" loading={saving}>
            Record payment
          </Button>
        </div>
      </form>
    </Modal>
  );
}
