"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";
import type { FrontendPlugin } from "@/types/api";
import { listPlugins } from "@/lib/api/plugins";

interface PluginContextValue {
  plugins: FrontendPlugin[];
  loading: boolean;
  error: string | null;
  isPluginEnabled: (name: string) => boolean;
  refresh: () => void;
}

const PluginContext = createContext<PluginContextValue>({
  plugins: [],
  loading: true,
  error: null,
  isPluginEnabled: () => false,
  refresh: () => {},
});

export function PluginProvider({ children }: { children: React.ReactNode }) {
  const [plugins, setPlugins] = useState<FrontendPlugin[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchPlugins = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const response = await listPlugins();
      setPlugins(response.plugins ?? []);
    } catch {
      // If the plugin endpoint is unavailable (e.g. auth not configured),
      // default to showing all navigation items.
      setPlugins([]);
      setError("Failed to load plugin configuration");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchPlugins();
  }, [fetchPlugins]);

  const isPluginEnabled = useCallback(
    (name: string): boolean => {
      // While loading or on error, default to showing items (graceful degradation).
      if (loading || error) {
        return true;
      }
      const plugin = plugins.find((p) => p.name === name);
      // If plugin is not in the list (unknown), show the item.
      if (!plugin) {
        return true;
      }
      return plugin.enabled;
    },
    [plugins, loading, error]
  );

  const value = useMemo(
    () => ({
      plugins,
      loading,
      error,
      isPluginEnabled,
      refresh: fetchPlugins,
    }),
    [plugins, loading, error, isPluginEnabled, fetchPlugins]
  );

  return (
    <PluginContext.Provider value={value}>{children}</PluginContext.Provider>
  );
}

export function usePlugins(): PluginContextValue {
  return useContext(PluginContext);
}
