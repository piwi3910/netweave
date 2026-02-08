"use client";

import { PluginGuard } from "@/components/shared/plugin-guard";

export default function OCloudsLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <PluginGuard
      pluginName="o2ims"
      displayName="O2-IMS Infrastructure Inventory"
    >
      {children}
    </PluginGuard>
  );
}
