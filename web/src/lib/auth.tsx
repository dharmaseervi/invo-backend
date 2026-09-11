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
  getToken,
  setToken,
  type AuthUser,
  type Company,
} from "./api";

type AuthState = {
  user: AuthUser | null;
  company: Company | null;
  companies: Company[];
  /** True until the stored token has been checked, so screens do not flash. */
  loading: boolean;
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

  const loadCompanies = useCallback(async () => {
    const res = await companiesApi.list();
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
      if (!getToken()) {
        setLoading(false);
        return;
      }
      try {
        const list = await loadCompanies();
        if (cancelled) return;
        setUser({ id: 0, email: "" });
        void list;
      } catch (err) {
        if (err instanceof ApiError && err.isAuthError) {
          setToken(null);
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
      signIn,
      signOut,
      selectCompany,
      refreshCompanies: loadCompanies,
    }),
    [user, companies, companyId, loading, signIn, signOut, selectCompany, loadCompanies],
  );

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useAuth() {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error("useAuth must be used inside AuthProvider");
  return ctx;
}

/** True once a token exists — the server is the real authority, this only gates the UI. */
export function useIsSignedIn() {
  const { loading } = useAuth();
  return { signedIn: !loading && Boolean(getToken()), loading };
}
