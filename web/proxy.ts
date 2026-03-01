import { NextRequest, NextResponse } from "next/server";

const PUBLIC_ROUTES = ["/login", "/api/auth", "/api/health"];

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

    // Auth.js uses __Secure- prefix on HTTPS; check both variants
    const sessionToken =
      request.cookies.get("authjs.session-token")?.value ??
      request.cookies.get("__Secure-authjs.session-token")?.value;

    const isPublicRoute = PUBLIC_ROUTES.some((route) =>
      pathname.startsWith(route),
    );

    // Redirect authenticated users away from login
    if (sessionToken && pathname === "/login") {
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
    // Set DISABLE_AUTH=1 to skip during local UI testing without OIDC
    if (!sessionToken && process.env.DISABLE_AUTH !== "1") {
      return NextResponse.redirect(new URL("/login", baseUrl));
    }

    return NextResponse.next();
  } catch (error) {
    console.error("Proxy error:", error);
    // Fail closed: deny access when the auth check itself fails
    return NextResponse.redirect(new URL("/login", request.url));
  }
}

export const config = {
  matcher: ["/((?!_next/static|_next/image|favicon.ico).*)"],
};
