// Typed client for the Go API.
//
// Served from the same origin, so requests are relative and CORS never applies. The
// token lives in localStorage because a static export has no server to set an
// httpOnly cookie — every value rendered from the API must therefore go through
// React's escaping, which it does, and none of it is ever passed to innerHTML.

// Relative in production: the Go server hosts the API and this app on one origin, so
// there is no CORS preflight and no host to configure. `next dev` serves the pages
// without the API, so NEXT_PUBLIC_API_ORIGIN points those requests at a locally running
// server — which then needs that dev origin in its own ALLOWED_ORIGINS. It is read at
// build time and must stay unset in the production build.
const BASE = (process.env.NEXT_PUBLIC_API_ORIGIN ?? "") + "/api/v1";
const TOKEN_KEY = "invo_token";

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }

  /** The server rejected the token; retrying will not help, the user must sign in. */
  get isAuthError() {
    return this.status === 401;
  }
}

export function getToken(): string | null {
  if (typeof window === "undefined") return null;
  try {
    return window.localStorage.getItem(TOKEN_KEY);
  } catch {
    // Private browsing and blocked site data both throw rather than return null.
    return null;
  }
}

export function setToken(token: string | null) {
  try {
    if (token) window.localStorage.setItem(TOKEN_KEY, token);
    else window.localStorage.removeItem(TOKEN_KEY);
  } catch {
    /* storage unavailable — the session simply will not survive a reload */
  }
}

type Options = {
  method?: string;
  body?: unknown;
  /** Endpoints that are reachable without a token (login, signup, reset). */
  auth?: boolean;
  signal?: AbortSignal;
  /** Extra headers — the client ledger takes its company this way rather than in the URL. */
  headers?: Record<string, string>;
};

export async function api<T>(path: string, opts: Options = {}): Promise<T> {
  const { method = "GET", body, auth = true, signal, headers: extra } = opts;

  const headers: Record<string, string> = { Accept: "application/json", ...extra };
  if (body !== undefined) headers["Content-Type"] = "application/json";

  if (auth) {
    const token = getToken();
    if (token) headers.Authorization = `Bearer ${token}`;
  }

  let res: Response;
  try {
    res = await fetch(`${BASE}${path}`, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
      signal,
    });
  } catch {
    // Distinguished from a server error, because the user's action differs: check
    // your connection, rather than something is wrong with the account.
    throw new ApiError(0, "Can't reach the server. Check your connection.");
  }

  if (res.status === 204) return undefined as T;

  const text = await res.text();
  let data: unknown = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = null;
    }
  }

  if (!res.ok) {
    // The API reports failures as {"error": "..."} — surface the server's own wording
    // rather than a generic message, since it is usually the actionable part.
    const message =
      (data && typeof data === "object" && "error" in data
        ? String((data as { error: unknown }).error)
        : "") || defaultMessage(res.status);
    throw new ApiError(res.status, message);
  }

  return data as T;
}

function defaultMessage(status: number): string {
  if (status === 401) return "Your session has expired. Please sign in again.";
  if (status === 403) return "You don't have access to that.";
  if (status === 404) return "Not found.";
  if (status === 429) return "Too many attempts. Please wait a moment.";
  if (status >= 500) return "Something went wrong on our end.";
  return "That didn't work.";
}

/* ---------- Auth ---------- */

export type AuthUser = { id: number; email: string };
type SessionResponse = { token: string; user: AuthUser };

export const auth = {
  login: (email: string, password: string) =>
    api<SessionResponse>("/login", {
      method: "POST",
      auth: false,
      body: { email, password },
    }),

  register: (email: string, password: string) =>
    api<{
      user_id: number;
      email: string;
      requires_verification: boolean;
      email_sent: boolean;
      message: string;
    }>("/register", { method: "POST", auth: false, body: { email, password } }),

  verifyEmail: (email: string, code: string) =>
    api<unknown>("/verify-email", {
      method: "POST",
      auth: false,
      body: { email, code },
    }),

  resendVerification: (email: string) =>
    api<unknown>("/resend-verification", {
      method: "POST",
      auth: false,
      body: { email },
    }),

  sendOtp: (email: string) =>
    api<unknown>("/send-otp", { method: "POST", auth: false, body: { email } }),

  verifyOtp: (email: string, code: string) =>
    api<SessionResponse>("/verify-otp", {
      method: "POST",
      auth: false,
      body: { email, code },
    }),

  forgotPassword: (email: string) =>
    api<unknown>("/forgot-password", {
      method: "POST",
      auth: false,
      body: { email },
    }),

  resetPassword: (email: string, code: string, newPassword: string) =>
    api<SessionResponse>("/reset-password", {
      method: "POST",
      auth: false,
      body: { email, code, new_password: newPassword },
    }),

  logout: () => api<unknown>("/logout", { method: "POST" }),
};

/* ---------- Companies ---------- */

export type Company = {
  id: number;
  name: string;
  address?: string;
  phone?: string;
  gst?: string;
  city?: string;
  state?: string;
  pincode?: string;
};

export const companies = {
  list: () => api<{ companies: Company[] }>("/companies"),
  create: (body: Omit<Company, "id">) =>
    api<{ company_id: number }>("/companies", { method: "POST", body }),
  /** These fields are what every invoice prints as the seller. */
  update: (id: number, body: Omit<Company, "id">) =>
    api<unknown>(`/companies/${id}`, { method: "PUT", body }),
};

/** Money as the API returns it, formatted the way an Indian invoice reads. */
export function formatMoney(value: number | null | undefined): string {
  const n = typeof value === "number" && Number.isFinite(value) ? value : 0;
  return n.toLocaleString("en-IN", {
    style: "currency",
    currency: "INR",
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
}

export function formatDate(value: string | null | undefined): string {
  if (!value) return "—";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleDateString("en-IN", {
    day: "2-digit",
    month: "short",
    year: "numeric",
  });
}

/* ---------- Clients ---------- */

export type Client = {
  id: number;
  name: string;
  email: string;
  phone: string;
  address: string;
  city: string;
  state: string;
  pincode: string;
};

/** The fields the create/update endpoints accept. */
export type ClientInput = Omit<Client, "id"> & { company_id: number };

export const clients = {
  /** `search` matches name, phone or email, and is applied by the server. */
  list: (companyId: number, search?: string, signal?: AbortSignal) =>
    api<{ clients: Client[] }>(
      `/companies/${companyId}/clients${search ? `?search=${encodeURIComponent(search)}` : ""}`,
      { signal },
    ),
  create: (body: ClientInput) =>
    api<{ client_id: number }>("/clients", { method: "POST", body }),
  update: (id: number, body: ClientInput) =>
    api<unknown>(`/clients/${id}`, { method: "PUT", body }),
  /** 409 when the client has invoices, payments or ledger history. */
  remove: (id: number) => api<unknown>(`/clients/${id}`, { method: "DELETE" }),
  /** Every invoice raised for this client, newest first. */
  invoices: (clientId: number) =>
    api<{ data: ClientInvoiceRow[] }>(`/clients/${clientId}/invoices`),
};

/** The per-client invoice list returns fewer columns than the main one. */
export type ClientInvoiceRow = {
  id: number;
  invoice_number: string;
  invoice_date: string;
  due_date: string;
  status: string;
  subtotal: number;
  tax: number;
  total: number;
  paid_amount: number;
  remaining_amount: number;
  created_at: string;
};

/**
 * A client's billing or shipping address, stored separately from the client row
 * because an invoice snapshots it — and because the GSTIN lives here, which is what
 * decides CGST/SGST against IGST on every invoice raised for them.
 */
export type ClientAddress = {
  type: "billing" | "shipping";
  name?: string | null;
  line1: string;
  line2?: string | null;
  city?: string | null;
  state?: string | null;
  postal_code?: string | null;
  country?: string | null;
  phone?: string | null;
  email?: string | null;
  gst_number?: string | null;
};

export const clientAddresses = {
  /** Returns `{ data: null }` when the client has no address of that type yet. */
  get: (clientId: number, type: "billing" | "shipping") =>
    api<{ data: ClientAddress | null }>(`/clients/${clientId}/address?type=${type}`),
  save: (clientId: number, body: ClientAddress) =>
    api<unknown>(`/clients/${clientId}/address`, { method: "POST", body }),
};

/* ---------- Categories ---------- */

export type Category = {
  id: number;
  name: string;
  user_id: number;
  company_id: number;
  default_hsn_code?: string | null;
  default_tax_rate?: number | null;
};

export const categories = {
  list: (companyId: number) =>
    api<{ categories: Category[] }>(`/categories/${companyId}`),
  create: (body: {
    name: string;
    company_id: number;
    default_hsn_code?: string | null;
    default_tax_rate?: number | null;
  }) => api<unknown>("/categories", { method: "POST", body }),
};

/* ---------- Items ---------- */

export type Item = {
  id: number;
  name: string;
  /** Null is meaningful: the item has no category. */
  category_id: number | null;
  sku: string;
  unit: string;
  description: string;
  cost_price: number;
  price: number;
  quantity: number;
  low_stock_alert: number;
  tax_rate: number;
  hsn_code: string;
  company_id: number;
  user_id: number;
};

export type ItemInput = Omit<Item, "id" | "user_id">;

export type StockMovement = {
  id: number;
  item_id: number;
  /** restock | sale | adjustment | initial */
  movement_type: string;
  quantity_change: number;
  previous_quantity: number;
  new_quantity: number;
  reference?: string | null;
  note?: string | null;
  created_at: string;
};

export const items = {
  /**
   * Keyset pagination. `cursor` is opaque — hand back whatever the last page
   * returned as `next_cursor`; omitting `limit` asks the server for everything,
   * which is what the catalogue screens deliberately avoid.
   */
  list: (
    companyId: number,
    opts: { limit?: number; cursor?: string; search?: string; signal?: AbortSignal } = {},
  ) => {
    const q = new URLSearchParams();
    if (opts.limit) q.set("limit", String(opts.limit));
    if (opts.cursor) q.set("cursor", opts.cursor);
    if (opts.search) q.set("search", opts.search);
    const qs = q.toString();
    return api<{ items: Item[]; next_cursor?: string }>(
      `/items/${companyId}/all${qs ? `?${qs}` : ""}`,
      { signal: opts.signal },
    );
  },
  create: (body: ItemInput) => api<unknown>("/items", { method: "POST", body }),
  update: (id: number, body: ItemInput) =>
    api<unknown>(`/items/${id}`, { method: "PUT", body }),
  /** Adds to stock and records why, as distinct from editing the quantity directly. */
  restock: (id: number, body: { quantity: number; reference?: string; note?: string }) =>
    api<{ quantity: number }>(`/item/${id}/restock`, { method: "POST", body }),
  movements: (id: number) =>
    api<{ movements: StockMovement[] }>(`/item/${id}/movements`),
};

/** Whole numbers, grouped the Indian way — quantities, not money. */
export function formatQty(value: number | null | undefined): string {
  const n = typeof value === "number" && Number.isFinite(value) ? value : 0;
  return n.toLocaleString("en-IN");
}

/* ---------- Invoices ---------- */

/** One line on an invoice, estimate or credit note as the API accepts it. */
export type LineInput = {
  item_id: number;
  qty: number;
  rate: number;
  discount: number;
  tax_rate: number;
};

export type InvoiceRow = {
  id: number;
  company_id: number;
  client_id: number;
  client_name: string;
  invoice_number: string;
  invoice_date: string;
  due_date: string;
  subtotal: number;
  tax: number;
  total: number;
  paid_amount: number;
  remaining_amount: number;
  status: string;
  is_overdue: boolean;
  days_overdue: number;
  created_at: string;
};

export type InvoiceLine = {
  id: number;
  item_id: number;
  /** Joined from the catalogue; empty when the item has since been deleted. */
  item_name: string;
  hsn_code: string;
  qty: number;
  rate: number;
  discount: number;
  tax_rate: number;
  total: number;
};

export type InvoiceDetail = {
  id: number;
  invoice_number: string;
  status: string;
  invoice_date: string;
  due_date: string;
  subtotal: number;
  tax: number;
  discount: number;
  total: number;
  paid_amount: number;
  remaining_amount: number;
  is_overdue: boolean;
  days_overdue: number;
  client: { id: number; name: string };
  items: InvoiceLine[];
  created_at: string;
};

export const invoices = {
  list: (
    companyId: number,
    opts: { limit?: number; offset?: number; clientId?: number; signal?: AbortSignal } = {},
  ) => {
    const q = new URLSearchParams({ company_id: String(companyId) });
    q.set("limit", String(opts.limit ?? 25));
    q.set("offset", String(opts.offset ?? 0));
    if (opts.clientId) q.set("client_id", String(opts.clientId));
    return api<{ data: InvoiceRow[]; limit: number; offset: number }>(
      `/invoices?${q}`,
      { signal: opts.signal },
    );
  },
  get: (id: number) => api<InvoiceDetail>(`/invoices/${id}`),
  create: (body: {
    company_id: number;
    client_id: number;
    invoice_date: string;
    due_date: string;
    notes?: string | null;
    discount: number;
    items: LineInput[];
  }) =>
    api<{ invoice_id: number; invoice_number: string }>("/invoices", {
      method: "POST",
      body,
    }),
  update: (
    id: number,
    body: {
      client_id: number;
      invoice_date: string;
      due_date: string;
      discount: number;
      items: LineInput[];
    },
  ) => api<unknown>(`/invoices/${id}/update`, { method: "PUT", body }),
  /**
   * Issuing assigns the number and moves stock, which is why it is separate from
   * saving a draft. `force` overrides the insufficient-stock refusal.
   */
  issue: (id: number, force = false) =>
    api<unknown>(`/invoices/${id}/issue${force ? "?force=true" : ""}`, { method: "POST" }),
  /** Cancelling keeps the record and reverses stock; deleting is only for drafts. */
  cancel: (id: number) => api<unknown>(`/invoices/${id}/cancel`, { method: "POST" }),
  remove: (id: number) => api<unknown>(`/invoices/${id}`, { method: "DELETE" }),
  numberPreview: (companyId: number) =>
    api<{ preview: string }>(`/invoices/number-preview?company_id=${companyId}`),
  /** Like the client ledger, this one takes its company from a header, not the query. */
  unpaid: (clientId: number, companyId: number) =>
    api<{ data: InvoiceRow[] }>(`/clients/${clientId}/unpaid-invoices`, {
      headers: { "X-Company-ID": String(companyId) },
    }),
  sendEmail: (id: number, body: { to_email: string; to_name: string; reminder?: boolean }) =>
    api<unknown>(`/invoices/${id}/send-email`, { method: "POST", body }),
};

/* ---------- Estimates ---------- */

export type EstimateRow = {
  id: number;
  client_id: number;
  client_name: string;
  estimate_number: string;
  estimate_date: string;
  expiry_date: string | null;
  subtotal: number;
  tax: number;
  discount: number;
  total: number;
  status: string;
  converted_invoice_id: number | null;
  created_at: string;
};

export type EstimateDetail = Omit<EstimateRow, "client_id" | "client_name"> & {
  client: { id: number; name: string };
  items: InvoiceLine[];
};

export const estimates = {
  list: (companyId: number, opts: { limit?: number; offset?: number; signal?: AbortSignal } = {}) => {
    const q = new URLSearchParams({ company_id: String(companyId) });
    q.set("limit", String(opts.limit ?? 50));
    q.set("offset", String(opts.offset ?? 0));
    return api<{ data: EstimateRow[] }>(`/estimates?${q}`, { signal: opts.signal });
  },
  get: (id: number) => api<EstimateDetail>(`/estimates/${id}`),
  create: (body: {
    company_id: number;
    client_id: number;
    estimate_date: string;
    expiry_date?: string | null;
    discount: number;
    items: LineInput[];
  }) =>
    api<{ estimate_id: number; estimate_number: string }>("/estimates", {
      method: "POST",
      body,
    }),
  update: (
    id: number,
    body: {
      client_id: number;
      estimate_date: string;
      expiry_date?: string | null;
      discount: number;
      items: LineInput[];
    },
  ) => api<unknown>(`/estimates/${id}/update`, { method: "PUT", body }),
  setStatus: (id: number, status: "sent" | "accepted" | "rejected") =>
    api<unknown>(`/estimates/${id}/status`, { method: "POST", body: { status } }),
  /** Creates a draft invoice from the estimate and links the two. */
  convert: (id: number) =>
    api<{ invoice_id: number; invoice_number: string }>(`/estimates/${id}/convert`, {
      method: "POST",
    }),
  numberPreview: (companyId: number) =>
    api<{ preview: string }>(`/estimates/number-preview?company_id=${companyId}`),
};

/* ---------- Payments ---------- */

export type PaymentRow = {
  id: number;
  client_id: number;
  client_name: string;
  amount: number;
  payment_method: string;
  reference: string;
  notes: string;
  payment_date: string;
  created_at: string;
  /** Invoice numbers this payment settled, comma separated. */
  applied_to: string;
};

export const payments = {
  list: (companyId: number, opts: { limit?: number; offset?: number } = {}) => {
    const q = new URLSearchParams();
    q.set("limit", String(opts.limit ?? 50));
    q.set("offset", String(opts.offset ?? 0));
    return api<{ payments: PaymentRow[] }>(`/companies/${companyId}/payments?${q}`);
  },
  /**
   * Allocations are optional — omitted, the server settles the client's oldest
   * open invoices first, which is what a shop actually does with a lump payment.
   */
  record: (body: {
    client_id: number;
    amount: number;
    payment_method: string;
    reference?: string;
    notes?: string;
    payment_date?: string;
    allocations?: { invoice_id: number; amount: number }[];
  }) => api<unknown>("/payments", { method: "POST", body }),
};

/* ---------- Expenses ---------- */

export type Expense = {
  id: number;
  company_id: number;
  name: string;
  amount: number;
  description: string;
  date: string;
  created_at: string;
  updated_at: string;
};

export const expenses = {
  list: (companyId: number, signal?: AbortSignal) =>
    api<{ expenses: Expense[] }>(`/companies/${companyId}/expenses`, { signal }),
  create: (body: {
    company_id: number;
    name: string;
    amount: number;
    description: string;
    date: string;
  }) => api<unknown>("/expenses", { method: "POST", body }),
  update: (
    id: number,
    body: { name: string; amount: number; description: string; date: string },
  ) => api<unknown>(`/expenses/${id}`, { method: "PUT", body }),
  remove: (id: number) => api<unknown>(`/expenses/${id}`, { method: "DELETE" }),
};

/* ---------- Credit notes ---------- */

export type CreditNoteType = "return" | "adjustment" | "discount";

export type CreditNoteRow = {
  id: number;
  credit_number: string;
  client_id: number;
  client_name: string;
  type: string;
  total: number;
  balance: number;
  status: string;
  credit_date: string;
};

export type CreditNoteDetail = {
  id: number;
  credit_number: string;
  client_id: number;
  client_name: string;
  invoice_id: number | null;
  invoice_number: string | null;
  type: string;
  reason: string | null;
  subtotal: number;
  tax: number;
  total: number;
  balance: number;
  status: string;
  credit_date: string;
  created_at: string;
  items: {
    id: number;
    item_id: number;
    item_name: string;
    qty: number;
    rate: number;
    tax_rate: number;
    total: number;
  }[];
};

export const creditNotes = {
  list: (companyId: number, signal?: AbortSignal) =>
    api<CreditNoteRow[]>(`/credit-notes?company_id=${companyId}`, { signal }),
  get: (id: number) => api<CreditNoteDetail>(`/credit-notes/${id}`),
  /**
   * `return` credits specific goods and puts them back into stock. `adjustment` and
   * `discount` credit an amount without touching stock — a correction after the fact,
   * and a discount allowed later. The service rejects anything else.
   */
  create: (body: {
    company_id: number;
    client_id: number;
    invoice_id?: number | null;
    type: CreditNoteType;
    credit_date: string;
    reason?: string;
    items?: { item_id: number; qty: number; rate: number; tax_rate: number }[];
    amount?: number;
  }) => api<unknown>("/credit-notes", { method: "POST", body }),
};

/* ---------- Ledger ---------- */

export type LedgerEntry = {
  id: number;
  client_id: number;
  client_name: string;
  source_type: string;
  source_id: number;
  debit: number;
  credit: number;
  balance: number;
  description: string;
  created_at: string;
};

export const ledger = {
  /** The client ledger takes its company from a header rather than the query string. */
  forClient: (clientId: number, companyId: number) =>
    api<{ data: LedgerEntry[] }>(`/ledger/${clientId}`, {
      headers: { "X-Company-ID": String(companyId) },
    }),
  forCompany: (companyId: number) =>
    api<{ data: LedgerEntry[] }>(`/companies/${companyId}/ledger`),
};

/* ---------- Dashboard ---------- */

export type Dashboard = {
  period: string;
  revenue: {
    total: number;
    change_percent: number;
    trend: { date: string; total: number }[];
  };
  counts: { invoices: number; clients: number; items: number };
  recent_invoices: {
    id: number;
    invoice_number: string;
    client_name: string;
    total: number;
    status: string;
    created_at: string;
  }[];
};

export const dashboard = {
  get: (companyId: number, period: "week" | "month" | "year" = "month") =>
    api<Dashboard>(`/dashboard?companyId=${companyId}&period=${period}`),
};

/* ---------- Reports ---------- */

export type StockReportItem = {
  id: number;
  name: string;
  sku: string;
  category_id: number | null;
  category_name: string;
  unit: string;
  quantity: number;
  cost_price: number;
  price: number;
  stock_value: number;
  retail_value: number;
  low_stock_alert: number;
  status: "in" | "low" | "out";
};

export type StockReport = {
  as_of: string;
  total_items: number;
  total_stock_units: number;
  total_cost_value: number;
  total_retail_value: number;
  potential_profit: number;
  low_stock_count: number;
  out_of_stock_count: number;
  items: StockReportItem[];
  categories: {
    category_id: number | null;
    category_name: string;
    item_count: number;
    total_units: number;
    cost_value: number;
    retail_value: number;
    potential_profit: number;
    low_stock_count: number;
    out_of_stock_count: number;
    share_of_value: number;
  }[];
};

export type AgingBucket = {
  current: number;
  days_1_30: number;
  days_31_60: number;
  days_61_90: number;
  days_90_plus: number;
};

export type AgingReport = {
  as_of: string;
  grand_total: number;
  totals: AgingBucket;
  clients: {
    client_id: number;
    client_name: string;
    total: number;
    buckets: AgingBucket;
  }[];
};

export type GSTSummary = {
  invoice_count: number;
  taxable_value: number;
  cgst: number;
  sgst: number;
  igst: number;
  total: number;
};

export type GSTReport = {
  start: string;
  end: string;
  company_state: string;
  summary: GSTSummary;
  net_summary: GSTSummary;
  invoices: {
    invoice_id: number;
    invoice_number: string;
    invoice_date: string;
    client_name: string;
    client_gstin: string;
    place_of_supply: string;
    taxable_value: number;
    cgst: number;
    sgst: number;
    igst: number;
    total: number;
  }[];
  hsn_summary: {
    hsn_code: string;
    tax_rate: number;
    total_qty: number;
    taxable_value: number;
    cgst: number;
    sgst: number;
    igst: number;
    total_value: number;
  }[];
  credit_notes: {
    credit_note_id: number;
    credit_number: string;
    credit_date: string;
    client_name: string;
    client_gstin: string;
    original_invoice: string;
    reason: string;
    taxable_value: number;
    cgst: number;
    sgst: number;
    igst: number;
    total: number;
  }[];
};

export const reports = {
  stock: (companyId: number) =>
    api<StockReport>(`/companies/${companyId}/reports/stock`),
  aging: (companyId: number) =>
    api<AgingReport>(`/companies/${companyId}/reports/aging`),
  gstr1: (companyId: number, start: string, end: string) =>
    api<GSTReport>(`/companies/${companyId}/reports/gstr1?start=${start}&end=${end}`),
};

/* ---------- Company settings ---------- */

export type CompanyBank = {
  id: number;
  company_id: number;
  account_holder_name: string;
  bank_name: string;
  account_number: string;
  ifsc_code: string;
  branch: string;
  upi_id: string;
  is_default: boolean;
};

export type BankInput = Omit<CompanyBank, "id" | "company_id">;

export const banks = {
  list: (companyId: number) => api<CompanyBank[]>(`/companies/${companyId}/banks`),
  // company_id goes in the body as well as the path: the handler authorises against
  // the body's value and ignores the URL parameter, so omitting it is a 403.
  create: (companyId: number, body: BankInput) =>
    api<CompanyBank>(`/companies/${companyId}/banks`, {
      method: "POST",
      body: { ...body, company_id: companyId },
    }),
  update: (companyId: number, bankId: number, body: BankInput) =>
    api<CompanyBank>(`/companies/${companyId}/banks/${bankId}`, {
      method: "PUT",
      body: { ...body, company_id: companyId },
    }),
};

export const profile = {
  get: () => api<{ user_id: number; email: string }>("/profile"),
};

/**
 * PDFs need the bearer token, so they cannot be opened as a plain link. Fetched as a
 * blob and handed to the browser as a download instead.
 */
export async function downloadPdf(path: string, filename: string): Promise<void> {
  const token = getToken();
  const res = await fetch(`${BASE}${path}`, {
    headers: token ? { Authorization: `Bearer ${token}` } : {},
  });
  if (!res.ok) {
    const text = await res.text();
    let message = "Failed to generate the PDF";
    try {
      const parsed = JSON.parse(text);
      if (parsed?.error) message = String(parsed.error);
    } catch {
      /* a non-JSON body means the generic message is the best available */
    }
    throw new ApiError(res.status, message);
  }
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

/** Today as YYYY-MM-DD in the user's own timezone, which is what date inputs expect. */
export function today(): string {
  const d = new Date();
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}
