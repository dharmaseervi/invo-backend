"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  ApiError,
  clientAddresses,
  clients as clientsApi,
  type Client,
  type ClientAddress,
} from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { INDIAN_STATES } from "@/lib/states";
import { AppShell, NoCompany } from "@/components/AppShell";
import {
  Button,
  Card,
  EmptyState,
  ErrorText,
  Field,
  IconButton,
  Modal,
  SearchInput,
  Select,
  SkeletonRows,
  useToast,
} from "@/components/ui";

export default function ClientsPage() {
  const { company, loading: authLoading } = useAuth();
  const toast = useToast();

  const [list, setList] = useState<Client[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [search, setSearch] = useState("");
  const [editing, setEditing] = useState<Client | null>(null);
  const [creating, setCreating] = useState(false);

  const companyId = company?.id ?? null;

  // Search runs on the server, so it is debounced — and every reply carries an
  // AbortController, otherwise a slow early response can land after a later one and
  // repopulate the list with results for a query the user has already moved past.
  const load = useCallback(
    async (term: string, signal: AbortSignal) => {
      if (!companyId) return;
      setLoading(true);
      setError("");
      try {
        const res = await clientsApi.list(companyId, term.trim() || undefined, signal);
        if (signal.aborted) return;
        setList(res.clients ?? []);
      } catch (err) {
        if (signal.aborted) return;
        setError(err instanceof ApiError ? err.message : "Failed to load clients");
      } finally {
        if (!signal.aborted) setLoading(false);
      }
    },
    [companyId],
  );

  useEffect(() => {
    if (!companyId) return;
    const controller = new AbortController();
    const t = window.setTimeout(() => void load(search, controller.signal), search ? 300 : 0);
    return () => {
      window.clearTimeout(t);
      controller.abort();
    };
  }, [companyId, search, load]);

  const reload = useCallback(() => {
    const controller = new AbortController();
    void load(search, controller.signal);
  }, [load, search]);

  const remove = useCallback(
    async (client: Client) => {
      if (
        !window.confirm(
          `Delete ${client.name}? This cannot be undone.`,
        )
      )
        return;
      try {
        await clientsApi.remove(client.id);
        toast.show(`${client.name} deleted`, "success");
        reload();
      } catch (err) {
        // 409 is the expected answer for a client with invoices or payments, and its
        // message explains what to do instead — so it is shown as-is.
        toast.show(
          err instanceof ApiError ? err.message : "Failed to delete client",
          "error",
        );
      }
    },
    [reload, toast],
  );

  if (authLoading || (!company && !authLoading)) {
    return (
      <AppShell title="Clients">
        {authLoading ? null : <NoCompany />}
      </AppShell>
    );
  }

  return (
    <AppShell
      title="Clients"
      description="The people and businesses you invoice"
      actions={
        <Button variant="primary" icon="plus" onClick={() => setCreating(true)}>
          Add client
        </Button>
      }
    >
      <div className="mb-4">
        <SearchInput
          value={search}
          onChange={setSearch}
          placeholder="Search name, phone or email"
        />
      </div>

      <Card>
        {loading && list.length === 0 ? (
          <SkeletonRows />
        ) : error ? (
          <EmptyState
            icon="alert"
            title="Couldn't load clients"
            message={error}
            action={<Button onClick={reload}>Try again</Button>}
          />
        ) : list.length === 0 ? (
          <EmptyState
            icon="client"
            title={search ? "No matches" : "No clients yet"}
            message={
              search
                ? `Nothing matched "${search}".`
                : "Add the people and businesses you invoice."
            }
            action={
              !search && (
                <Button variant="primary" icon="plus" onClick={() => setCreating(true)}>
                  Add client
                </Button>
              )
            }
          />
        ) : (
          <ul className="divide-y divide-line">
            {list.map((client) => (
              <li
                key={client.id}
                className="group flex flex-wrap items-center gap-x-4 gap-y-2 px-5 py-3.5 transition hover:bg-subtle/60"
              >
                <span className="grid h-9 w-9 shrink-0 place-items-center rounded-full bg-accent-soft text-[13px] font-semibold uppercase text-accent">
                  {client.name.trim().charAt(0) || "?"}
                </span>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{client.name}</p>
                  <p className="mt-0.5 truncate text-sm text-muted">
                    {[client.phone, client.email].filter(Boolean).join(" · ") ||
                      "No contact details"}
                  </p>
                </div>
                <p className="ml-auto hidden truncate text-sm text-muted sm:block sm:w-48">
                  {[client.city, client.state].filter(Boolean).join(", ") || "—"}
                </p>
                <div className="flex items-center gap-1">
                  <IconButton icon="edit" label={`Edit ${client.name}`} onClick={() => setEditing(client)} />
                  <IconButton
                    icon="trash"
                    label={`Delete ${client.name}`}
                    onClick={() => void remove(client)}
                    className="hover:text-danger"
                  />
                </div>
              </li>
            ))}
          </ul>
        )}
      </Card>

      {list.length > 0 && (
        <p className="mt-3 text-xs text-muted">
          {list.length} client{list.length === 1 ? "" : "s"}
        </p>
      )}

      {(creating || editing) && companyId && (
        <ClientForm
          companyId={companyId}
          client={editing}
          onClose={() => {
            setCreating(false);
            setEditing(null);
          }}
          onSaved={(name) => {
            setCreating(false);
            setEditing(null);
            toast.show(`${name} saved`, "success");
            reload();
          }}
        />
      )}
    </AppShell>
  );
}

/* ---------- Create / edit ---------- */

const BLANK = {
  name: "",
  email: "",
  phone: "",
  address: "",
  city: "",
  state: "",
  pincode: "",
};

function ClientForm({
  companyId,
  client,
  onClose,
  onSaved,
}: {
  companyId: number;
  client: Client | null;
  onClose: () => void;
  onSaved: (name: string) => void;
}) {
  const [form, setForm] = useState(() =>
    client
      ? {
          name: client.name,
          email: client.email,
          phone: client.phone,
          address: client.address,
          city: client.city,
          state: client.state,
          pincode: client.pincode,
        }
      : BLANK,
  );
  const [gstin, setGstin] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  // The GSTIN lives on the client's billing address, not the client row, so it is
  // fetched separately. A client with no address yet simply has none.
  useEffect(() => {
    if (!client) return;
    let cancelled = false;
    (async () => {
      try {
        const res = await clientAddresses.get(client.id, "billing");
        if (cancelled) return;
        setGstin(res.data?.gst_number ?? "");
      } catch {
        /* an address is optional; leaving the field blank is correct */
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [client]);

  const set = (key: keyof typeof BLANK) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm((f) => ({ ...f, [key]: e.target.value }));

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    const name = form.name.trim();
    if (!name) {
      setError("Client name is required");
      return;
    }
    setSaving(true);
    setError("");
    try {
      const body = { ...form, name, company_id: companyId };
      let clientId = client?.id;
      if (client) {
        await clientsApi.update(client.id, body);
      } else {
        const created = await clientsApi.create(body);
        clientId = created.client_id;
      }

      // The billing address is kept in step with the client on every save, not only
      // when the GSTIN changes. An invoice snapshots this address and the server
      // refuses to raise one without it, so a client saved without an address here
      // could never be invoiced.
      const line1 = form.address.trim();
      if (clientId && line1) {
        const address: ClientAddress = {
          type: "billing",
          name,
          line1,
          city: form.city,
          state: form.state,
          postal_code: form.pincode,
          country: "India",
          phone: form.phone,
          email: form.email,
          gst_number: gstin.trim() || null,
        };
        await clientAddresses.save(clientId, address);
      } else if (clientId && gstin.trim()) {
        setError(
          "Saved, but a street address is needed before the GSTIN can be stored.",
        );
        setSaving(false);
        return;
      }

      onSaved(name);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to save client");
      setSaving(false);
    }
  };

  const stateOptions = useMemo(() => INDIAN_STATES, []);

  return (
    <Modal open title={client ? "Edit client" : "Add client"} onClose={onClose}>
      <form onSubmit={submit} noValidate>
        <ErrorText>{error}</ErrorText>

        <p className="mb-3 text-xs font-medium text-muted">Contact information</p>
        <Field label="Full name" value={form.name} onChange={set("name")} autoFocus />
        <Field
          label="Email"
          type="email"
          value={form.email}
          onChange={set("email")}
          placeholder="name@company.com"
        />
        <Field label="Phone" value={form.phone} onChange={set("phone")} />

        <p className="mb-3 mt-6 text-xs font-medium text-muted">Address</p>
        <Field
          label="Street address"
          value={form.address}
          onChange={set("address")}
          hint="Required before this client can be invoiced"
        />
        <div className="grid gap-x-4 sm:grid-cols-2">
          <Field label="City" value={form.city} onChange={set("city")} />
          <Select
            label="State"
            value={form.state}
            onChange={(e) => setForm((f) => ({ ...f, state: e.target.value }))}
            hint="Decides CGST/SGST or IGST on their invoices"
          >
            <option value="">Select state</option>
            {stateOptions.map((s) => (
              <option key={s} value={s}>
                {s}
              </option>
            ))}
          </Select>
        </div>
        <Field label="Pincode" value={form.pincode} onChange={set("pincode")} />

        <Field
            label="GSTIN"
            value={gstin}
            onChange={(e) => setGstin(e.target.value.toUpperCase())}
            placeholder="29ABCDE1234F1Z5"
            hint="Optional. Required on their invoices if they claim input credit."
          />

        <div className="mt-6 flex justify-end gap-2">
          <Button type="button" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" variant="primary" loading={saving}>
            {client ? "Save changes" : "Add client"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
