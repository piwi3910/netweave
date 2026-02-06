import apiClient from "./client";
import type { Role, Permission } from "@/types/api";

export async function listRoles(): Promise<Role[]> {
  return apiClient.get<Role[]>("/roles");
}

export async function getRole(roleId: string): Promise<Role> {
  return apiClient.get<Role>(`/roles/${roleId}`);
}

export async function listPermissions(): Promise<Permission[]> {
  return apiClient.get<Permission[]>("/permissions");
}
