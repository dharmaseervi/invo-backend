"use client";

import { useCallback, useEffect, useState } from "react";
import {
  ApiError,
  banks as banksApi,
  companies as companiesApi,
  profile as profileApi,
  type Company,
  type CompanyBank,
} from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { INDIAN_STATES } from "@/lib/states";
import { AppShell } from "@/components/AppShell";
import {
  Badge,
  Button,
  Card,
  ErrorText,
  Field,
  Modal,
  Select,
  Spinner,
  useToast,
} from "@/components/ui";

export default function SettingsPage() {
  const { company, companies, loading: authLoading, refreshCompanies } = useAuth();
  const toast = useToast();
  const [email, setEmail] = useState("");

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const res = await profileApi.get();
        if (!cancelled) setEmail(res.email ?? "");
      } catch {
        /* the account section simply shows nothing */
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  if (authLoading) {
    return (
      <AppShell title="Settings">
        <Spinner />
      </AppShell>
    );
  }

  return (
    <AppShell title="Settings">
      <div className="space-y-5">
        <Card className="p-5">
          <p className="text-sm font-medium">Account</p>
          <p className="mt-1 text-sm text-muted">{email || "—"}</p>
        </Card>

        {companies.length === 0 ? (
          <CompanyForm
            company={null}
            onSaved={async () => {
              await refreshCompanies();
              toast.show("Company created", "success");
            }}
          />
        ) : (
          <>
            <CompanyForm
              key={company?.id ?? "new"}
              company={company}
              onSaved={async () => {
                await refreshCompanies();
                toast.show("Company saved", "success");
              }}
            />
            {company && <BanksCard companyId={company.id} />}
          </>
        )}
      </div>
    </AppShell>
  );
}

/**
 * Company details — the seller block printed on every invoice, read by the PDF
 * straight from this row. State is not cosmetic: it is compared against the client's
 * place of supply to decide CGST+SGST against IGST.
 */
function CompanyForm({
  company,
  onSaved,
}: {
  company: Company | null;
  onSaved: () => Promise<void>;
}) {
  const [form, setForm] = useState({
    name: company?.name ?? "",
    address: company?.address ?? "",
    city: company?.city ?? "",
    state: company?.state ?? "",
    pincode: company?.pincode ?? "",
    phone: company?.phone ?? "",
    gst: company?.gst ?? "",
  });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  const set = (key: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm((f) => ({ ...f, [key]: e.target.value }));

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!form.name.trim()) {
      setError("Company name is required");
      return;
    }
    setSaving(true);
    setError("");
    try {
      const body = { ...form, name: form.name.trim() };
      if (company) await companiesApi.update(company.id, body);
      else await companiesApi.create(body);
      await onSaved();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to save the company");
    } finally {
      setSaving(false);
    }
  };

  return (
    <Card className="p-5">
      <p className="mb-4 text-sm font-medium">
        {company ? "Company" : "Create your company"}
      </p>
      <form onSubmit={submit} noValidate>
        <ErrorText>{error}</ErrorText>
        <Field label="Name" value={form.name} onChange={set("name")} />
        <div className="grid gap-x-4 sm:grid-cols-2">
          <Field label="GSTIN" value={form.gst} onChange={set("gst")} placeholder="29ABCDE1234F1Z5" />
          <Field label="Phone" value={form.phone} onChange={set("phone")} />
        </div>
        <Field label="Address" value={form.address} onChange={set("address")} />
        <div className="grid gap-x-4 sm:grid-cols-3">
          <Field label="City" value={form.city} onChange={set("city")} />
          <Select
            label="State"
            value={form.state}
            onChange={(e) => setForm((f) => ({ ...f, state: e.target.value }))}
            hint="Decides CGST/SGST vs IGST"
          >
            <option value="">Select state</option>
            {INDIAN_STATES.map((st) => (
              <option key={st} value={st}>
                {st}
              </option>
            ))}
          </Select>
          <Field label="Pincode" value={form.pincode} onChange={set("pincode")} />
        </div>
        <p className="mb-4 text-label-12 text-muted">
          These details print on every invoice as the seller.
        </p>
        <div className="flex justify-end">
          <Button type="submit" variant="primary" loading={saving}>
            {company ? "Save changes" : "Create company"}
          </Button>
        </div>
      </form>
    </Card>
  );
}

/** Bank details printed on the invoice so customers know where to pay. */
function BanksCard({ companyId }: { companyId: number }) {
  const toast = useToast();
  const [list, setList] = useState<CompanyBank[]>([]);
  const [loading, setLoading] = useState(true);
  const [editing, setEditing] = useState<CompanyBank | null>(null);
  const [adding, setAdding] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setList((await banksApi.list(companyId)) ?? []);
    } catch {
      setList([]);
    } finally {
      setLoading(false);
    }
  }, [companyId]);

  useEffect(() => {
    void (async () => {
      await load();
    })();
  }, [load]);

  return (
    <Card className="p-5">
      <div className="mb-4 flex items-center justify-between">
        <p className="text-sm font-medium">Bank accounts</p>
        <Button onClick={() => setAdding(true)}>Add account</Button>
      </div>

      {loading ? (
        <Spinner />
      ) : list.length === 0 ? (
        <p className="py-6 text-center text-sm text-muted">
          No bank account yet. Adding one puts your payment details on every invoice.
        </p>
      ) : (
        <ul className="divide-y divide-line">
          {list.map((b) => (
            <li key={b.id} className="flex items-center gap-4 py-3">
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <p className="truncate text-label-14 font-medium">{b.bank_name}</p>
                  {b.is_default && <Badge tone="success">Default</Badge>}
                </div>
                <p className="mt-0.5 truncate text-copy-13 text-muted">
                  {[b.account_number, b.ifsc_code, b.branch].filter(Boolean).join(" · ")}
                </p>
              </div>
              <Button onClick={() => setEditing(b)}>Edit</Button>
            </li>
          ))}
        </ul>
      )}

      {(adding || editing) && (
        <BankForm
          companyId={companyId}
          bank={editing}
          onClose={() => {
            setAdding(false);
            setEditing(null);
          }}
          onSaved={() => {
            setAdding(false);
            setEditing(null);
            toast.show("Bank account saved", "success");
            void load();
          }}
        />
      )}
    </Card>
  );
}

function BankForm({
  companyId,
  bank,
  onClose,
  onSaved,
}: {
  companyId: number;
  bank: CompanyBank | null;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [form, setForm] = useState({
    account_holder_name: bank?.account_holder_name ?? "",
    bank_name: bank?.bank_name ?? "",
    account_number: bank?.account_number ?? "",
    ifsc_code: bank?.ifsc_code ?? "",
    branch: bank?.branch ?? "",
    upi_id: bank?.upi_id ?? "",
    is_default: bank?.is_default ?? false,
  });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  const set = (key: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm((f) => ({ ...f, [key]: e.target.value }));

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!form.bank_name.trim() || !form.account_number.trim()) {
      setError("Bank name and account number are required");
      return;
    }
    setSaving(true);
    setError("");
    try {
      if (bank) await banksApi.update(companyId, bank.id, form);
      else await banksApi.create(companyId, form);
      onSaved();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to save the account");
      setSaving(false);
    }
  };

  return (
    <Modal open title={bank ? "Edit bank account" : "Add bank account"} onClose={onClose}>
      <form onSubmit={submit} noValidate>
        <ErrorText>{error}</ErrorText>
        <Field
          label="Account holder"
          value={form.account_holder_name}
          onChange={set("account_holder_name")}
          autoFocus
        />
        <Field label="Bank name" value={form.bank_name} onChange={set("bank_name")} />
        <div className="grid gap-x-4 sm:grid-cols-2">
          <Field label="Account number" value={form.account_number} onChange={set("account_number")} />
          <Field
            label="IFSC code"
            value={form.ifsc_code}
            onChange={(e) =>
              setForm((f) => ({ ...f, ifsc_code: e.target.value.toUpperCase() }))
            }
          />
        </div>
        <div className="grid gap-x-4 sm:grid-cols-2">
          <Field label="Branch" value={form.branch} onChange={set("branch")} />
          <Field label="UPI ID" value={form.upi_id} onChange={set("upi_id")} />
        </div>
        <label className="mb-4 flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={form.is_default}
            onChange={(e) => setForm((f) => ({ ...f, is_default: e.target.checked }))}
          />
          Use this account on invoices
        </label>
        <div className="flex justify-end gap-2">
          <Button type="button" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" variant="primary" loading={saving}>
            Save
          </Button>
        </div>
      </form>
    </Modal>
  );
}
