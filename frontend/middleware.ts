import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

const API_TARGET = process.env.INTERNAL_API_BASE_URL ?? "http://127.0.0.1:8000";

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

  // Auth gate — redirect unauthenticated users to /login
  const token = request.cookies.get("auth_token");
  if (!token) {
    return NextResponse.redirect(new URL("/login", request.url));
  }
  return NextResponse.next();
}

export const config = {
  matcher: [
    "/api/:path*",
    "/((?!login|_next/static|_next/image|favicon.ico).*)",
  ],
};
