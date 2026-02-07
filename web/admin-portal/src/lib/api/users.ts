import apiClient from "./client";
import type {
  TenantUser,
  CreateUserRequest,
  UpdateUserRequest,
} from "@/types/api";

export async function listUsers(): Promise<TenantUser[]> {
  return apiClient.get<TenantUser[]>("/tenant/users");
}

export async function getUser(userId: string): Promise<TenantUser> {
  return apiClient.get<TenantUser>(`/tenant/users/${userId}`);
}

export async function createUser(
  data: CreateUserRequest
): Promise<TenantUser> {
  return apiClient.post<TenantUser>("/tenant/users", data);
}

export async function updateUser(
  userId: string,
  data: UpdateUserRequest
): Promise<TenantUser> {
  return apiClient.put<TenantUser>(`/tenant/users/${userId}`, data);
}

export async function deleteUser(userId: string): Promise<void> {
  return apiClient.delete(`/tenant/users/${userId}`);
}

export async function getCurrentUser(): Promise<TenantUser> {
  return apiClient.get<TenantUser>("/tenant/me");
}
