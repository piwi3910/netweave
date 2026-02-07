"use client";

import { useEffect } from "react";
import { useSession } from "next-auth/react";
import { apiClient } from "@/lib/api/client";
import { adminApiClient } from "@/lib/api/admin-client";

export function ApiProvider({ children }: { children: React.ReactNode }) {
  const { data: session } = useSession();

  useEffect(() => {
    const token =
      (session as unknown as { accessToken?: string } | null)?.accessToken ??
      null;

    const tokenProvider = async () => token;

    apiClient.setTokenProvider(tokenProvider);
    adminApiClient.setTokenProvider(tokenProvider);
  }, [session]);

  return <>{children}</>;
}
