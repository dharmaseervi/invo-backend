"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  ApiError,
  expenses as expensesApi,
  formatDate,
  formatMoney,
  today,
  type Expense,
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
  Spinner,
  TextArea,
  useToast,
} from "@/components/ui";

export default function ExpensesPage() {
  const { company, loading: authLoading } = useAuth();
  const toast = useToast();

  const [rows, setRows] = useState<Expense[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [editing, setEditing] = useState<Expense | null>(null);
  const [creating, setCreating] = useState(false);

  const companyId = company?.id ?? null;

  const load = useCallback(async () => {
    if (!companyId) return;
    setLoading(true);
    setError("");
    try {
      const res = await expensesApi.list(companyId);
      setRows(res.expenses ?? []);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to load expenses");
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

  const total = useMemo(() => rows.reduce((sum, e) => sum + (e.amount ?? 0), 0), [rows]);

  const remove = async (expense: Expense) => {
    if (!window.confirm(`Delete "${expense.name}"? This cannot be undone.`)) return;
    try {
      await expensesApi.remove(expense.id);
      toast.show("Expense deleted", "success");
      void load();
    } catch (err) {
      toast.show(err instanceof ApiError ? err.message : "Failed to delete", "error");
    }
  };

  if (authLoading || !company) {
    return (
      <AppShell title="Expenses">{authLoading ? <Spinner /> : <NoCompany />}</AppShell>
    );
  }

  return (
    <AppShell
      title="Expenses"
      actions={
        <Button variant="primary" onClick={() => setCreating(true)}>
          Add expense
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
            title="Couldn't load expenses"
            message={error}
            action={<Button onClick={() => void load()}>Try again</Button>}
          />
        ) : rows.length === 0 ? (
          <EmptyState
            title="No expenses yet"
            message="Track what the business spends, so profit is more than sales minus guesswork."
            action={
              <Button variant="primary" onClick={() => setCreating(true)}>
                Add expense
              </Button>
            }
          />
        ) : (
          <ul className="divide-y divide-line">
            {rows.map((e) => (
              <li key={e.id} className="flex flex-wrap items-center gap-x-4 gap-y-2 px-5 py-4">
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{e.name}</p>
                  <p className="mt-0.5 truncate text-sm text-muted">
                    {[formatDate(e.date), e.description].filter(Boolean).join(" · ")}
                  </p>
                </div>
                <p className="tabular w-28 text-right text-sm font-medium">
                  {formatMoney(e.amount)}
                </p>
                <div className="flex gap-2">
                  <Button onClick={() => setEditing(e)}>Edit</Button>
                  <Button variant="danger" onClick={() => void remove(e)}>
                    Delete
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </Card>

      {rows.length > 0 && (
        <p className="mt-3 text-right text-sm">
          <span className="text-muted">Total </span>
          <span className="tabular font-medium">{formatMoney(total)}</span>
        </p>
      )}

      {(creating || editing) && (
        <ExpenseForm
          companyId={company.id}
          expense={editing}
          onClose={() => {
            setCreating(false);
            setEditing(null);
          }}
          onSaved={(name) => {
            setCreating(false);
            setEditing(null);
            toast.show(`${name} saved`, "success");
            void load();
          }}
        />
      )}
    </AppShell>
  );
}

function ExpenseForm({
  companyId,
  expense,
  onClose,
  onSaved,
}: {
  companyId: number;
  expense: Expense | null;
  onClose: () => void;
  onSaved: (name: string) => void;
}) {
  const [name, setName] = useState(expense?.name ?? "");
  const [amount, setAmount] = useState(String(expense?.amount ?? ""));
  const [date, setDate] = useState(expense?.date ?? today());
  const [description, setDescription] = useState(expense?.description ?? "");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = name.trim();
    const value = Number(amount);
    if (!trimmed) {
      setError("Name is required");
      return;
    }
    if (!Number.isFinite(value) || value < 0) {
      setError("Amount must be zero or more");
      return;
    }
    if (!date) {
      setError("Date is required");
      return;
    }
    setSaving(true);
    setError("");
    try {
      const body = { name: trimmed, amount: value, description: description.trim(), date };
      if (expense) await expensesApi.update(expense.id, body);
      else await expensesApi.create({ ...body, company_id: companyId });
      onSaved(trimmed);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to save expense");
      setSaving(false);
    }
  };

  return (
    <Modal open title={expense ? "Edit expense" : "Add expense"} onClose={onClose}>
      <form onSubmit={submit} noValidate>
        <ErrorText>{error}</ErrorText>
        <Field
          label="Name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Shop rent, electricity, transport"
          autoFocus
        />
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
            label="Date"
            type="date"
            value={date}
            onChange={(e) => setDate(e.target.value)}
          />
        </div>
        <TextArea
          label="Description"
          rows={2}
          value={description}
          onChange={(e) => setDescription(e.target.value)}
        />
        <div className="mt-4 flex justify-end gap-2">
          <Button type="button" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" variant="primary" loading={saving}>
            {expense ? "Save changes" : "Add expense"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
