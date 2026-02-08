export interface Tenant {
  id: string;
  name: string;
  description: string;
  status: "active" | "pending_deletion" | "inactive";
  quota: TenantQuota;
  usage: TenantUsage;
  contactEmail: string;
  metadata: Record<string, string>;
  createdAt: string;
  updatedAt: string;
}

export interface TenantQuota {
  maxSubscriptions: number;
  maxResourcePools: number;
  maxResources: number;
  maxUsers: number;
}

export interface TenantUsage {
  subscriptions: number;
  resourcePools: number;
  resources: number;
  users: number;
}

export interface TenantUser {
  id: string;
  tenantId: string;
  subject: string;
  commonName: string;
  email: string;
  roleId: string;
  isActive: boolean;
}

export interface CreateUserRequest {
  subject: string;
  commonName: string;
  email?: string;
  roleId: string;
  isActive?: boolean;
}

export interface UpdateUserRequest {
  email?: string;
  roleId?: string;
  isActive?: boolean;
}

export interface Role {
  id: string;
  name: string;
  description: string;
  type: "platform" | "tenant";
  tenantId: string;
  permissions: Permission[];
}

export interface Permission {
  id: string;
  name: string;
  description: string;
  resource: string;
  action: string;
}

export interface CreateTenantRequest {
  name: string;
  description?: string;
  contactEmail: string;
  quota?: Partial<TenantQuota>;
  metadata?: Record<string, string>;
}

export interface UpdateTenantRequest {
  name?: string;
  description?: string;
  contactEmail?: string;
  metadata?: Record<string, string>;
}

export interface Certificate {
  serialNumber: string;
  commonName: string;
  userId: string;
  tenantId: string;
  certificatePEM: string;
  issuingCA: string;
  caChain: string[];
  issuedAt: string;
  expiresAt: string;
  ttl: string;
  revokedAt: string | null;
  status:
    | "active"
    | "expiring_soon"
    | "expired"
    | "revoked"
    | "renewal_pending"
    | "renewal_failed";
  renewalAttempts: number;
  lastRenewalAttempt: string | null;
}

export interface CertificateRequest {
  commonName: string;
  altNames?: string[];
  ipSANs?: string[];
  ttl?: string;
}

export interface MonitoringReport {
  totalCertificates: number;
  activeCertificates: number;
  expiringSoon: number;
  expired: number;
  revoked: number;
  renewalsPending: number;
  renewalsFailed: number;
  nextScanTime: string;
}

export interface AuditEvent {
  id: string;
  type: string;
  tenantId: string;
  userId: string;
  subject: string;
  resourceType: string;
  resourceId: string;
  action: string;
  details: Record<string, string>;
  clientIP: string;
  userAgent: string;
  timestamp: string;
}

export interface Subscription {
  subscriptionId: string;
  callback: string;
  consumerSubscriptionId: string;
  filter: {
    resourcePoolId: string[];
    resourceTypeId: string[];
    resourceId: string[];
  };
}

export interface FrontendPlugin {
  name: string;
  displayName: string;
  enabled: boolean;
  basePaths: string[];
}

export interface PluginListResponse {
  plugins: FrontendPlugin[];
}

export interface ApiError {
  error: string;
  message: string;
  code: number;
}

export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  limit: number;
  offset: number;
}
