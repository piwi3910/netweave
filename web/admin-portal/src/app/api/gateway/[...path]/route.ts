import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth/config";
import { sanitizeError } from "@/lib/utils/sanitize-error";

const GATEWAY_URL =
  process.env.GATEWAY_INTERNAL_URL || "https://localhost:8080";

const PROXY_TIMEOUT_MS = Number(process.env.PROXY_TIMEOUT_MS) || 30_000;

const ALLOWED_CONTENT_TYPES = [
  "application/json",
  "application/x-www-form-urlencoded",
  "text/plain",
];

let gatewayOrigin: string;
try {
  gatewayOrigin = new URL(GATEWAY_URL).origin;
} catch {
  throw new Error(`Invalid GATEWAY_INTERNAL_URL: ${GATEWAY_URL}`);
}

function isValidPathSegment(segment: string): boolean {
  return segment !== ".." && segment !== "." && !/[\x00]/.test(segment);
}

function errorResponse(
  error: string,
  message: string,
  code: number,
): NextResponse {
  return NextResponse.json({ error, message, code }, { status: code });
}

function buildTargetUrl(targetPath: string, search: string): string | null {
  const raw = `${GATEWAY_URL}${targetPath}${search}`;
  try {
    const parsed = new URL(raw);
    if (parsed.origin !== gatewayOrigin) {
      return null;
    }
    return parsed.toString();
  } catch {
    return null;
  }
}

async function proxyRequest(
  req: NextRequest,
  { params }: { params: Promise<{ path: string[] }> },
): Promise<NextResponse> {
  const session = await auth();
  if (!session) {
    return errorResponse("Unauthorized", "Not authenticated", 401);
  }

  if (session.error === "RefreshTokenError") {
    return errorResponse("Unauthorized", "Session expired", 401);
  }

  const { accessToken } = session;
  if (!accessToken) {
    return errorResponse("Unauthorized", "No access token", 401);
  }

  const { path } = await params;

  if (!path.every(isValidPathSegment)) {
    return errorResponse("BadRequest", "Invalid path", 400);
  }

  const targetPath = "/" + path.join("/");
  const search = req.nextUrl.search;
  const targetUrl = buildTargetUrl(targetPath, search);

  if (!targetUrl) {
    console.error("[proxy] Invalid target URL", req.method, targetPath);
    return errorResponse("BadRequest", "Invalid request URL", 400);
  }

  const headers: HeadersInit = {
    Authorization: `Bearer ${accessToken}`,
  };

  const contentType = req.headers.get("content-type");
  if (contentType) {
    const baseType = contentType.split(";")[0].trim().toLowerCase();
    if (ALLOWED_CONTENT_TYPES.includes(baseType)) {
      headers["Content-Type"] = contentType;
    }
  }

  const fetchOptions: RequestInit = {
    method: req.method,
    headers,
    signal: AbortSignal.timeout(PROXY_TIMEOUT_MS),
  };

  if (req.method !== "GET" && req.method !== "HEAD") {
    fetchOptions.body = await req.text();
  }

  let response: Response;
  try {
    response = await fetch(targetUrl, fetchOptions);
  } catch (err) {
    if (err instanceof DOMException && err.name === "TimeoutError") {
      console.error("[proxy] Gateway timeout", req.method, targetPath);
      return errorResponse(
        "GatewayTimeout",
        "Gateway request timed out",
        504,
      );
    }
    console.error(
      "[proxy] Gateway connection error",
      req.method,
      targetPath,
      err instanceof Error ? err.message : String(err),
    );
    return errorResponse("BadGateway", "Failed to reach gateway", 502);
  }

  const responseHeaders = new Headers();
  const forwardHeaders = ["content-type", "x-request-id"];
  for (const header of forwardHeaders) {
    const value = response.headers.get(header);
    if (value) {
      responseHeaders.set(header, value);
    }
  }

  if (response.status === 204) {
    return new NextResponse(null, {
      status: 204,
      headers: responseHeaders,
    });
  }

  const body = await response.text();

  if (response.ok) {
    return new NextResponse(body, {
      status: response.status,
      headers: responseHeaders,
    });
  }

  // Log all error responses for debugging and correlation
  console.error(
    "[proxy]",
    response.status,
    req.method,
    targetPath,
    response.headers.get("x-request-id") ?? "-",
  );

  // For error responses, sanitize the body before forwarding to prevent
  // leaking internal details (token fragments, stack traces, file paths).
  let sanitizedBody: string;
  try {
    const parsed = JSON.parse(body);
    if (typeof parsed.message === "string") {
      parsed.message = sanitizeError(parsed.message);
    }
    sanitizedBody = JSON.stringify({
      error: parsed.error || "Error",
      message: parsed.message || response.statusText,
      code: parsed.code || response.status,
    });
    responseHeaders.set("content-type", "application/json");
  } catch {
    sanitizedBody = JSON.stringify({
      error: "Error",
      message: sanitizeError(body),
      code: response.status,
    });
    responseHeaders.set("content-type", "application/json");
  }

  return new NextResponse(sanitizedBody, {
    status: response.status,
    headers: responseHeaders,
  });
}

export const GET = proxyRequest;
export const POST = proxyRequest;
export const PUT = proxyRequest;
export const DELETE = proxyRequest;
export const PATCH = proxyRequest;
