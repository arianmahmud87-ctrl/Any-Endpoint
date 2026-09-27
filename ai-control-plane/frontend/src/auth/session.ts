import createContextHook from "@nkzw/create-context-hook";
import { useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useState } from "react";
import { api, PREVIEW_MODE, setCsrfToken, setUnauthorizedHandler } from "@/lib/api/client";
import { useMe } from "@/lib/api/hooks";
import { ApiError } from "@/lib/api/types";

export interface RecentRun {
  id: string;
  title: string;
}

export const [SessionProvider, useSession] = createContextHook(() => {
  const qc = useQueryClient();
  const me = useMe();
  const [expired, setExpired] = useState<boolean>(false);
  const [recentRuns, setRecentRuns] = useState<RecentRun[]>([]);

  useEffect(() => {
    setCsrfToken(me.data?.csrf_token ?? null);
  }, [me.data]);

  useEffect(() => {
    setUnauthorizedHandler(() => setExpired(true));
  }, []);

  const status: "loading" | "authed" | "anon" | "forbidden" | "error" = me.isLoading
    ? "loading"
    : me.data
      ? "authed"
      : me.error instanceof ApiError && me.error.status === 401
        ? "anon"
        : me.error instanceof ApiError && me.error.status === 403
          ? "forbidden"
          : "error";

  const login = useCallback(async () => {
    if (PREVIEW_MODE) {
      await api("/api/auth/preview_login", { method: "POST" });
      setExpired(false);
      await qc.invalidateQueries({ queryKey: ["me"] });
      return;
    }
    window.location.assign("/api/auth/google/start");
  }, [qc]);

  const clear = useCallback(() => {
    setCsrfToken(null);
    setRecentRuns([]);
    qc.clear();
    qc.setQueryData(["me"], undefined);
  }, [qc]);

  const logout = useCallback(async (all = false) => {
    try {
      await api(all ? "/api/auth/sessions/revoke_all" : "/api/auth/logout", { method: "POST" });
    } catch {
      /* session may already be gone */
    }
    clear();
    await qc.invalidateQueries({ queryKey: ["me"] });
  }, [clear, qc]);

  const addRun = useCallback((r: RecentRun) => setRecentRuns((p) => [r, ...p].slice(0, 12)), []);

  return { me: me.data, status, refetch: me.refetch, expired, login, logout, recentRuns, addRun };
});
