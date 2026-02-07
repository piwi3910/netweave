import { ApiClient } from "./client";

const ADMIN_API_BASE_URL =
  process.env.NEXT_PUBLIC_ADMIN_API_BASE_URL ||
  "http://localhost:8080/api/v1/admin";

export const adminApiClient = new ApiClient(ADMIN_API_BASE_URL);
export default adminApiClient;
