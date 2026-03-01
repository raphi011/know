import type { MetadataRoute } from "next";
import { env } from "@/app/lib/env";

export default function sitemap(): MetadataRoute.Sitemap {
  const baseUrl = env.APP_URL;

  return [
    {
      url: baseUrl,
      lastModified: new Date(),
      changeFrequency: "daily",
      priority: 1,
    },
    {
      url: `${baseUrl}/docs`,
      lastModified: new Date(),
      changeFrequency: "daily",
      priority: 0.8,
    },
  ];
}
