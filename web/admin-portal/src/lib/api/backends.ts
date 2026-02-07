import adminApiClient from "./admin-client";
import type {
  BackendResponse,
  BackendsListResponse,
  CreateBackendRequest,
  UpdateBackendRequest,
  TestBackendResult,
  BackendAccess,
  BackendAccessListResponse,
  CreateBackendAccessRequest,
  UpdateBackendAccessRequest,
  AdapterTypeSchema,
  AdapterTypesListResponse,
  CreateDMSLinkRequest,
} from "@/types/backends";

export async function listBackends(
  category?: string
): Promise<BackendResponse[]> {
  const params = category ? { category } : undefined;
  const response = await adminApiClient.get<BackendsListResponse>(
    "/backends",
    params
  );
  return response.backends;
}

export async function getBackend(id: string): Promise<BackendResponse> {
  return adminApiClient.get<BackendResponse>(`/backends/${id}`);
}

export async function createBackend(
  data: CreateBackendRequest
): Promise<BackendResponse> {
  return adminApiClient.post<BackendResponse>("/backends", data);
}

export async function updateBackend(
  id: string,
  data: UpdateBackendRequest
): Promise<BackendResponse> {
  return adminApiClient.put<BackendResponse>(`/backends/${id}`, data);
}

export async function deleteBackend(id: string): Promise<void> {
  await adminApiClient.delete<void>(`/backends/${id}`);
}

export async function testBackend(id: string): Promise<TestBackendResult> {
  return adminApiClient.post<TestBackendResult>(`/backends/${id}/test`, {});
}

export async function listDMSLinks(imsId: string): Promise<BackendResponse[]> {
  const response = await adminApiClient.get<BackendsListResponse>(
    `/backends/${imsId}/dms-links`
  );
  return response.backends;
}

export async function createDMSLink(
  imsId: string,
  dmsId: string
): Promise<void> {
  const data: CreateDMSLinkRequest = { imsId, dmsId };
  await adminApiClient.post<void>(`/backends/${imsId}/dms-links`, data);
}

export async function deleteDMSLink(
  imsId: string,
  dmsId: string
): Promise<void> {
  await adminApiClient.delete<void>(`/backends/${imsId}/dms-links/${dmsId}`);
}

export async function listBackendAccess(
  tenantId: string
): Promise<BackendAccess[]> {
  const response = await adminApiClient.get<BackendAccessListResponse>(
    `/tenants/${tenantId}/backend-access`
  );
  return response.access;
}

export async function createBackendAccess(
  tenantId: string,
  data: CreateBackendAccessRequest
): Promise<BackendAccess> {
  return adminApiClient.post<BackendAccess>(
    `/tenants/${tenantId}/backend-access`,
    data
  );
}

export async function updateBackendAccess(
  tenantId: string,
  accessId: string,
  data: UpdateBackendAccessRequest
): Promise<BackendAccess> {
  return adminApiClient.put<BackendAccess>(
    `/tenants/${tenantId}/backend-access/${accessId}`,
    data
  );
}

export async function deleteBackendAccess(
  tenantId: string,
  accessId: string
): Promise<void> {
  await adminApiClient.delete<void>(
    `/tenants/${tenantId}/backend-access/${accessId}`
  );
}

export async function listBackendTypes(
  category?: string
): Promise<AdapterTypeSchema[]> {
  const params = category ? { category } : undefined;
  const response = await adminApiClient.get<AdapterTypesListResponse>(
    "/backend-types",
    params
  );
  return response.types;
}

export async function getBackendTypeSchema(
  type: string
): Promise<AdapterTypeSchema> {
  return adminApiClient.get<AdapterTypeSchema>(`/backend-types/${type}/schema`);
}
