import { ApiClient } from "./client";

function resolveAdminBaseUrl(): string {
  if (process.env.NEXT_PUBLIC_ADMIN_API_BASE_URL) {
    return process.env.NEXT_PUBLIC_ADMIN_API_BASE_URL;
  }
  // Derive from the main API base URL by replacing the path suffix.
  const apiBase =
    process.env.NEXT_PUBLIC_API_BASE_URL ||
    "http://localhost:8080/o2ims-infrastructureInventory/v1";
  try {
    const url = new URL(apiBase);
    url.pathname = "/api/v1/admin";
    return url.toString().replace(/\/$/, "");
  } catch {
    return "http://localhost:8080/api/v1/admin";
  }
}

export const adminApiClient = new ApiClient(resolveAdminBaseUrl());
export default adminApiClient;
