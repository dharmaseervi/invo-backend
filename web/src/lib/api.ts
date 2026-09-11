// Typed client for the Go API.
//
// Served from the same origin, so requests are relative and CORS never applies. The
// token lives in localStorage because a static export has no server to set an
// httpOnly cookie — every value rendered from the API must therefore go through
// React's escaping, which it does, and none of it is ever passed to innerHTML.

const BASE = "/api/v1";
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
