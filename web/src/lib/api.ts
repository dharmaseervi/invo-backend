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
};

export async function api<T>(path: string, opts: Options = {}): Promise<T> {
  const { method = "GET", body, auth = true, signal } = opts;

  const headers: Record<string, string> = { Accept: "application/json" };
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
  create: (body: ClientInput) => api<unknown>("/clients", { method: "POST", body }),
  update: (id: number, body: ClientInput) =>
    api<unknown>(`/clients/${id}`, { method: "PUT", body }),
  /** 409 when the client has invoices, payments or ledger history. */
  remove: (id: number) => api<unknown>(`/clients/${id}`, { method: "DELETE" }),
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
