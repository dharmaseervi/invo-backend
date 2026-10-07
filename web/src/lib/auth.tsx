"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";
import { useRouter } from "next/navigation";
import {
  ApiError,
  auth as authApi,
  companies as companiesApi,
  setToken,
  type AuthUser,
  type Company,
} from "./api";

type AuthState = {
  user: AuthUser | null;
  company: Company | null;
  companies: Company[];
  /** True until the session has been checked with the server, so screens do not flash. */
  loading: boolean;
  /**
   * Whether the server accepted this browser's session.
   *
   * Not inferred from a token here: in production the session is an httpOnly cookie
   * the page cannot read, so the only way to know is to have used it.
   */
  signedIn: boolean;
  /**
   * Set when the company list could not be loaded — a dropped connection, a server
   * error — as opposed to an account that genuinely has no company yet. Without it both
   * look identical, and a shop with ten years of invoices was invited to "create your
   * first company" because one request failed.
   */
  companiesError: string | null;
  signIn: (token: string, user: AuthUser) => Promise<void>;
  signOut: () => Promise<void>;
  selectCompany: (id: number) => void;
  refreshCompanies: () => Promise<Company[]>;
};

const Ctx = createContext<AuthState | null>(null);
const COMPANY_KEY = "invo_company";

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const [user, setUser] = useState<AuthUser | null>(null);
  const [companies, setCompanies] = useState<Company[]>([]);
  const [companyId, setCompanyId] = useState<number | null>(null);
  const [loading, setLoading] = useState(true);
  const [companiesError, setCompaniesError] = useState<string | null>(null);
  const [signedIn, setSignedIn] = useState(false);

  const loadCompanies = useCallback(async () => {
    let res;
    try {
      res = await companiesApi.list();
    } catch (err) {
      // An auth failure is handled by the caller, which signs the session out; anything
      // else is reported so the screen can offer Retry instead of pretending the
      // account is empty.
      if (!(err instanceof ApiError && err.isAuthError)) {
        setCompaniesError(err instanceof Error ? err.message : "Could not load your companies.");
      }
      throw err;
    }
    setCompaniesError(null);
    const list = res.companies ?? [];
    setCompanies(list);

    // Keep the remembered company only if it still belongs to this account —
    // otherwise a stale id from a previous login silently scopes every request.
    let stored: number | null = null;
    try {
      const raw = window.localStorage.getItem(COMPANY_KEY);
      stored = raw ? Number(raw) : null;
    } catch {
      stored = null;
    }
    const valid = list.some((c) => c.id === stored) ? stored : (list[0]?.id ?? null);
    setCompanyId(valid);
    try {
      if (valid) window.localStorage.setItem(COMPANY_KEY, String(valid));
    } catch {
      /* storage unavailable */
    }
    return list;
  }, []);

  // Restore the session on first load. A token that the server no longer accepts —
  // expired, or revoked by a logout or password reset elsewhere — is discarded here
  // rather than failing on whichever screen the user happens to open.
  useEffect(() => {
    let cancelled = false;

    (async () => {
      // Asked of the server rather than inferred from a token here. In production
      // the session is an httpOnly cookie this page cannot read, so "is there a
      // token?" has no answer worth having — the only way to know whether the cookie
      // is still good is to use it. A 401 means no session, which is the same answer
      // the old check gave for a missing token, and also catches the cases it never
      // did: expired, revoked by a logout elsewhere, or reset on another device.
      try {
        const list = await loadCompanies();
        if (cancelled) return;
        setUser({ id: 0, email: "" });
        setSignedIn(true);
        void list;
      } catch (err) {
        if (err instanceof ApiError && err.isAuthError) {
          setToken(null);
          if (!cancelled) setSignedIn(false);
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [loadCompanies]);

  const signIn = useCallback(
    async (token: string, nextUser: AuthUser) => {
      setToken(token);
      setUser(nextUser);
      setSignedIn(true);
      try {
        await loadCompanies();
      } catch {
        // A new account has no company yet; onboarding handles that.
        setCompanies([]);
      }
    },
    [loadCompanies],
  );

  const signOut = useCallback(async () => {
    try {
      // Server-side revocation, so a copy of the token taken from this browser
      // stops working rather than remaining valid until it expires.
      await authApi.logout();
    } catch {
      /* signing out must succeed locally even if the call fails */
    }
    setToken(null);
    setUser(null);
    setSignedIn(false);
    setCompanies([]);
    setCompanyId(null);
    try {
      window.localStorage.removeItem(COMPANY_KEY);
    } catch {
      /* storage unavailable */
    }
    router.replace("/login");
  }, [router]);

  const selectCompany = useCallback((id: number) => {
    setCompanyId(id);
    try {
      window.localStorage.setItem(COMPANY_KEY, String(id));
    } catch {
      /* storage unavailable */
    }
  }, []);

  const value = useMemo<AuthState>(
    () => ({
      user,
      companies,
      company: companies.find((c) => c.id === companyId) ?? null,
      loading,
      signedIn,
      companiesError,
      signIn,
      signOut,
      selectCompany,
      refreshCompanies: loadCompanies,
    }),
    [
      user,
      companies,
      companyId,
      loading,
      signedIn,
      companiesError,
      signIn,
      signOut,
      selectCompany,
      loadCompanies,
    ],
  );

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useAuth() {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error("useAuth must be used inside AuthProvider");
  return ctx;
}

/** True once the server has accepted this browser's session. Gates the UI only. */
export function useIsSignedIn() {
  const { loading, signedIn } = useAuth();
  return { signedIn: !loading && signedIn, loading };
}
