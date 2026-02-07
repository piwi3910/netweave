export interface BackendResponse {
  id: string;
  name: string;
  category: string;
  adapterType: string;
  description: string;
  status: string;
  statusMessage: string;
  hasCredentials: boolean;
  lastHealthCheck: string;
  createdAt: string;
  updatedAt: string;
}

export interface BackendAccess {
  id: string;
  tenantId: string;
  backendId: string;
  permissions: string[];
  constraints: Record<string, unknown>;
  grantedAt: string;
  grantedBy: string;
}

export interface FieldSpec {
  name: string;
  label: string;
  type: "string" | "number" | "boolean" | "text";
  required: boolean;
  secret: boolean;
  default: string;
  placeholder: string;
  regex: string;
  description: string;
}

export interface AdapterTypeSchema {
  type: string;
  category: string;
  displayName: string;
  description: string;
  configSchema: FieldSpec[];
  credentialSchema: FieldSpec[];
}

export interface CreateBackendRequest {
  name: string;
  category: string;
  adapterType: string;
  description?: string;
  config?: Record<string, string>;
  credentials?: Record<string, string>;
}

export interface UpdateBackendRequest {
  name?: string;
  description?: string;
  config?: Record<string, string>;
  credentials?: Record<string, string>;
}

export interface TestBackendResult {
  backendId: string;
  status: string;
  message: string;
}

export interface BackendsListResponse {
  backends: BackendResponse[];
  total: number;
}

export interface BackendAccessListResponse {
  access: BackendAccess[];
  total: number;
}

export interface AdapterTypesListResponse {
  types: AdapterTypeSchema[];
}

export interface CreateDMSLinkRequest {
  imsId: string;
  dmsId: string;
}

export interface CreateBackendAccessRequest {
  backendId: string;
  permissions: string[];
  constraints?: Record<string, unknown>;
}

export interface UpdateBackendAccessRequest {
  permissions?: string[];
  constraints?: Record<string, unknown>;
}
