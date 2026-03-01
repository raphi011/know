import type { MetadataRoute } from "next";
import { env } from "@/app/lib/env";

export default function robots(): MetadataRoute.Robots {
  const baseUrl = env.APP_URL;

  return {
    rules: {
      userAgent: "*",
      allow: "/",
      disallow: ["/api/", "/login"],
    },
    sitemap: `${baseUrl}/sitemap.xml`,
  };
}
