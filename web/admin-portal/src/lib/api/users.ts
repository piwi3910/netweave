import apiClient from "./client";
import type {
  TenantUser,
  CreateUserRequest,
  UpdateUserRequest,
} from "@/types/api";

export async function listUsers(): Promise<TenantUser[]> {
  return apiClient.get<TenantUser[]>("/users");
}

export async function getUser(userId: string): Promise<TenantUser> {
  return apiClient.get<TenantUser>(`/users/${userId}`);
}

export async function createUser(
  data: CreateUserRequest
): Promise<TenantUser> {
  return apiClient.post<TenantUser>("/users", data);
}

export async function updateUser(
  userId: string,
  data: UpdateUserRequest
): Promise<TenantUser> {
  return apiClient.put<TenantUser>(`/users/${userId}`, data);
}

export async function deleteUser(userId: string): Promise<void> {
  return apiClient.delete(`/users/${userId}`);
}

export async function getCurrentUser(): Promise<TenantUser> {
  return apiClient.get<TenantUser>("/user");
}
