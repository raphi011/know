import { NextRequest, NextResponse } from "next/server";

const SESSION_COOKIE = "kh_session";
const PUBLIC_ROUTES = ["/login", "/api/health"];

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

    // Allow static files and Next.js internals
    if (
      pathname.startsWith("/_next") ||
      pathname.startsWith("/favicon") ||
      pathname.includes(".")
    ) {
      return NextResponse.next();
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
      return NextResponse.redirect(new URL(destination, baseUrl));
    }

    // Allow public routes
    if (isPublicRoute) {
      return NextResponse.next();
    }

    // Redirect unauthenticated users to login
    if (!hasSession) {
      return NextResponse.redirect(new URL("/login", baseUrl));
    }

    return NextResponse.next();
  } catch (error) {
    console.error("Proxy error:", error);
    return NextResponse.redirect(new URL("/login", request.url));
  }
}

export const config = {
  matcher: ["/((?!_next/static|_next/image|favicon.ico).*)"],
};
