import apiClient from "./client";
import type {
  Tenant,
  TenantQuota,
  CreateTenantRequest,
  UpdateTenantRequest,
} from "@/types/api";

export async function listTenants(): Promise<Tenant[]> {
  return apiClient.get<Tenant[]>("/platform/tenants");
}

export async function getTenant(tenantId: string): Promise<Tenant> {
  return apiClient.get<Tenant>(`/platform/tenants/${tenantId}`);
}

export async function createTenant(
  data: CreateTenantRequest
): Promise<Tenant> {
  return apiClient.post<Tenant>("/platform/tenants", data);
}

export async function updateTenant(
  tenantId: string,
  data: UpdateTenantRequest
): Promise<Tenant> {
  return apiClient.put<Tenant>(`/platform/tenants/${tenantId}`, data);
}

export async function deleteTenant(tenantId: string): Promise<void> {
  return apiClient.delete(`/platform/tenants/${tenantId}`);
}

export async function getTenantQuotas(
  tenantId: string
): Promise<TenantQuota> {
  return apiClient.get<TenantQuota>(`/platform/tenants/${tenantId}/quotas`);
}

export async function updateTenantQuotas(
  tenantId: string,
  quotas: Partial<TenantQuota>
): Promise<TenantQuota> {
  return apiClient.put<TenantQuota>(`/platform/tenants/${tenantId}/quotas`, quotas);
}
