import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/lib/auth/config";

const GATEWAY_URL =
  process.env.GATEWAY_INTERNAL_URL || "https://localhost:8080";

async function proxyRequest(
  req: NextRequest,
  { params }: { params: Promise<{ path: string[] }> }
): Promise<NextResponse> {
  const session = await auth();
  if (!session) {
    return NextResponse.json(
      { error: "Unauthorized", message: "Not authenticated", code: 401 },
      { status: 401 }
    );
  }

  const accessToken = (session as unknown as { accessToken?: string })
    ?.accessToken;
  if (!accessToken) {
    return NextResponse.json(
      { error: "Unauthorized", message: "No access token", code: 401 },
      { status: 401 }
    );
  }

  const { path } = await params;
  const targetPath = "/" + path.join("/");
  const search = req.nextUrl.search;
  const targetUrl = `${GATEWAY_URL}${targetPath}${search}`;

  const headers: HeadersInit = {
    Authorization: `Bearer ${accessToken}`,
  };

  const contentType = req.headers.get("content-type");
  if (contentType) {
    headers["Content-Type"] = contentType;
  }

  const fetchOptions: RequestInit = {
    method: req.method,
    headers,
  };

  if (req.method !== "GET" && req.method !== "HEAD") {
    fetchOptions.body = await req.text();
  }

  const response = await fetch(targetUrl, fetchOptions);

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
  return new NextResponse(body, {
    status: response.status,
    headers: responseHeaders,
  });
}

export const GET = proxyRequest;
export const POST = proxyRequest;
export const PUT = proxyRequest;
export const DELETE = proxyRequest;
export const PATCH = proxyRequest;
