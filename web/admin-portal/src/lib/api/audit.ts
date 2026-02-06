import apiClient from "./client";
import type { AuditEvent } from "@/types/api";

export interface AuditQueryParams {
  limit?: number;
  offset?: number;
  tenantId?: string;
}

export async function listAuditEvents(
  params?: AuditQueryParams
): Promise<AuditEvent[]> {
  const queryParams: Record<string, string> = {};
  if (params?.limit) queryParams.limit = String(params.limit);
  if (params?.offset) queryParams.offset = String(params.offset);
  if (params?.tenantId) queryParams.tenantId = params.tenantId;
  return apiClient.get<AuditEvent[]>("/audit/events", queryParams);
}

export async function listAuditEventsByType(
  eventType: string,
  limit?: number
): Promise<AuditEvent[]> {
  const params: Record<string, string> = {};
  if (limit) params.limit = String(limit);
  return apiClient.get<AuditEvent[]>(
    `/audit/events/type/${eventType}`,
    params
  );
}
