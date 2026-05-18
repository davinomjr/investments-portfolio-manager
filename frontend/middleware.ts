import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

const API_TARGET = process.env.INTERNAL_API_BASE_URL ?? "http://127.0.0.1:8000";

// Treat the token as expired this many milliseconds before its real expiry to
// absorb modest clock skew between the browser and the backend that signed it.
const EXPIRY_SKEW_MS = 30_000;

function isAuthTokenUsable(token: string | undefined): boolean {
  if (!token) return false;
  const parts = token.split(".");
  if (parts.length !== 3) return false;
  try {
    const payload = parts[1].replace(/-/g, "+").replace(/_/g, "/");
    const padded = payload + "=".repeat((4 - (payload.length % 4)) % 4);
    const claims = JSON.parse(atob(padded)) as { exp?: number };
    if (typeof claims.exp !== "number") return false;
    return Date.now() + EXPIRY_SKEW_MS < claims.exp * 1000;
  } catch {
    return false;
  }
}

export async function middleware(request: NextRequest) {
  // Proxy /api/* requests to the backend
  if (request.nextUrl.pathname.startsWith("/api/")) {
    const backendPath = request.nextUrl.pathname.replace(/^\/api/, "");
    const url = `${API_TARGET}${backendPath}${request.nextUrl.search}`;
    const headers = new Headers(request.headers);
    headers.delete("host");

    const res = await fetch(url, {
      method: request.method,
      headers,
      body: request.method !== "GET" && request.method !== "HEAD" ? request.body : undefined,
      // @ts-expect-error -- Node fetch supports duplex for streaming bodies
      duplex: "half",
    });

    const responseHeaders = new Headers(res.headers);
    responseHeaders.delete("transfer-encoding");

    return new NextResponse(res.body, {
      status: res.status,
      statusText: res.statusText,
      headers: responseHeaders,
    });
  }

  // Auth gate — redirect to /login if the cookie is missing or its JWT is expired.
  // NextResponse.redirect does not auto-prepend basePath, so build the path manually.
  const tokenCookie = request.cookies.get("auth_token");
  if (!isAuthTokenUsable(tokenCookie?.value)) {
    const basePath = process.env.NEXT_PUBLIC_BASE_PATH ?? "";
    const response = NextResponse.redirect(new URL(`${basePath}/login`, request.url));
    if (tokenCookie) {
      response.cookies.delete("auth_token");
    }
    return response;
  }
  return NextResponse.next();
}

export const config = {
  matcher: [
    "/api/:path*",
    "/((?!login|_next/static|_next/image|favicon.ico).*)",
  ],
};
