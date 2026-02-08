"use client";

import { usePlugins } from "@/lib/contexts/plugin-context";

interface PluginGuardProps {
  pluginName: string;
  displayName: string;
  children: React.ReactNode;
}

export function PluginGuard({
  pluginName,
  displayName,
  children,
}: PluginGuardProps) {
  const { isPluginEnabled, loading } = usePlugins();

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary" />
      </div>
    );
  }

  if (!isPluginEnabled(pluginName)) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="text-center space-y-4 max-w-md">
          <h2 className="text-xl font-semibold text-foreground">
            {displayName} is not enabled
          </h2>
          <p className="text-muted-foreground">
            This API plugin is currently disabled. Contact your platform
            administrator to enable the {displayName} plugin.
          </p>
        </div>
      </div>
    );
  }

  return <>{children}</>;
}
