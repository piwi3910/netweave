import apiClient from "./client";
import type { Role, Permission } from "@/types/api";

export async function listRoles(): Promise<Role[]> {
  return apiClient.get<Role[]>("/tenant/roles");
}

export async function getRole(roleId: string): Promise<Role> {
  return apiClient.get<Role>(`/tenant/roles/${roleId}`);
}

export async function listPermissions(): Promise<Permission[]> {
  return apiClient.get<Permission[]>("/tenant/permissions");
}
