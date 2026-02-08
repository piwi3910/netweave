import apiClient from "./client";
import type { PluginListResponse } from "@/types/api";

export async function listPlugins(): Promise<PluginListResponse> {
  return apiClient.get<PluginListResponse>("/platform/plugins");
}
