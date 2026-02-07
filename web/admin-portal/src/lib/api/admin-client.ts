import { ApiClient } from "./client";

// Infrastructure admin routes go through the server-side proxy.
// The gateway serves these at /admin/infrastructure/*.
export const adminApiClient = new ApiClient("/api/gateway/admin/infrastructure");
export default adminApiClient;
