import { NextRequest, NextResponse } from "next/server";

const SESSION_COOKIE = "kh_session";
const PUBLIC_ROUTES = ["/login", "/api/health"];

export function buildCsp(nonce: string, isDev: boolean): string {
  return [
    "default-src 'self'",
    `script-src 'self' 'nonce-${nonce}' 'strict-dynamic'${isDev ? " 'unsafe-eval'" : ""}`,
    "style-src 'self' 'unsafe-inline'",
    "img-src 'self' blob: data:",
    "font-src 'self'",
    "connect-src 'self'",
    "frame-ancestors 'none'",
    "base-uri 'self'",
    "object-src 'none'",
  ].join("; ");
}

function withCsp(response: NextResponse, csp: string): NextResponse {
  response.headers.set("Content-Security-Policy", csp);
  return response;
}

function nextWithNonce(
  request: NextRequest,
  nonce: string,
  csp: string,
): NextResponse {
  const requestHeaders = new Headers(request.headers);
  requestHeaders.set("x-nonce", nonce);
  requestHeaders.set("Content-Security-Policy", csp);
  const response = NextResponse.next({ request: { headers: requestHeaders } });
  return withCsp(response, csp);
}

export function isValidReturnTo(returnTo: string): boolean {
  try {
    const decoded = decodeURIComponent(returnTo);
    return (
      decoded.startsWith("/") &&
      !decoded.startsWith("//") &&
      !decoded.includes(":") &&
      !decoded.includes("\\") &&
      !decoded.includes("\n") &&
      !decoded.includes("\r")
    );
  } catch {
    return false;
  }
}

export async function proxy(request: NextRequest) {
  try {
    const { pathname } = request.nextUrl;
    const baseUrl = process.env.APP_URL || request.url;

    const nonce = Buffer.from(crypto.randomUUID()).toString("base64");
    const csp = buildCsp(nonce, process.env.NODE_ENV === "development");

    // Allow static files and Next.js internals
    if (
      pathname.startsWith("/_next") ||
      pathname.startsWith("/favicon") ||
      pathname.includes(".")
    ) {
      return nextWithNonce(request, nonce, csp);
    }

    const hasSession = request.cookies.has(SESSION_COOKIE);

    const isPublicRoute = PUBLIC_ROUTES.some((route) =>
      pathname.startsWith(route),
    );

    // Redirect authenticated users away from login
    if (hasSession && pathname === "/login") {
      const returnTo = request.nextUrl.searchParams.get("returnTo");
      const destination =
        returnTo && isValidReturnTo(returnTo) ? returnTo : "/docs";
      return withCsp(
        NextResponse.redirect(new URL(destination, baseUrl)),
        csp,
      );
    }

    // Allow public routes
    if (isPublicRoute) {
      return nextWithNonce(request, nonce, csp);
    }

    // Redirect unauthenticated users to login
    if (!hasSession) {
      return withCsp(
        NextResponse.redirect(new URL("/login", baseUrl)),
        csp,
      );
    }

    return nextWithNonce(request, nonce, csp);
  } catch (error) {
    console.error("Proxy error:", error);
    return NextResponse.redirect(new URL("/login", request.url));
  }
}

export const config = {
  matcher: [
    {
      source: "/((?!_next/static|_next/image|favicon.ico).*)",
      missing: [
        { type: "header", key: "next-router-prefetch" },
        { type: "header", key: "purpose", value: "prefetch" },
      ],
    },
  ],
};
