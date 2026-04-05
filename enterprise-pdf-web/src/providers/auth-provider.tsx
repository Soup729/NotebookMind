"use client";

import { apiRequest, normalizeToken } from "@/lib/api";
import type { AuthResponse, User } from "@/types/api";
import { createContext, ReactNode, useContext, useEffect, useMemo, useState } from "react";

type AuthContextValue = {
  token: string;
  user: User | null;
  ready: boolean;
  login: (payload: AuthResponse) => void;
  logout: () => void;
};

const STORAGE_KEY = "enterprise-pdf-auth";

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [token, setToken] = useState("");
  const [user, setUser] = useState<User | null>(null);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) {
      setReady(true);
      return;
    }

    try {
      const parsed = JSON.parse(raw) as AuthResponse;
      setToken(normalizeToken(parsed.token));
      setUser(parsed.user);
    } catch {
      window.localStorage.removeItem(STORAGE_KEY);
    } finally {
      setReady(true);
    }
  }, []);

  useEffect(() => {
    if (!ready || !token) {
      return;
    }

    apiRequest<{ user: User }>("/me", {}, token)
      .then((response) => setUser(response.user))
      .catch(() => {
        setToken("");
        setUser(null);
        window.localStorage.removeItem(STORAGE_KEY);
      });
  }, [ready, token]);

  const value = useMemo<AuthContextValue>(
    () => ({
      token,
      user,
      ready,
      login: (payload) => {
        const normalized = normalizeToken(payload.token);
        setToken(normalized);
        setUser(payload.user);
        window.localStorage.setItem(STORAGE_KEY, JSON.stringify({ ...payload, token: normalized }));
      },
      logout: () => {
        setToken("");
        setUser(null);
        window.localStorage.removeItem(STORAGE_KEY);
      }
    }),
    [ready, token, user]
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error("useAuth must be used inside AuthProvider");
  }
  return context;
}
