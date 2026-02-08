"use client";

import { PluginGuard } from "@/components/shared/plugin-guard";

export default function DMSLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <PluginGuard pluginName="o2dms" displayName="O2-DMS Deployment Management">
      {children}
    </PluginGuard>
  );
}
