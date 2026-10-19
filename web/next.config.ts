import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  /* config options here */
  distDir: process.env.NODE_ENV === "development" ? ".next-dev" : ".next",
  cacheComponents: true,
  partialPrefetching: true,
  // Optional same-origin API proxy. When API_PROXY_TARGET is set (for example
  // to https://<vm-ip>.nip.io on Vercel) and NEXT_PUBLIC_API_BASE_URL is empty,
  // the browser calls /api/v1/* on the web origin and Next forwards it to the
  // gateway. The refresh cookie then becomes first-party, so SameSite=Lax works
  // even though the API lives on a different site.
  async rewrites() {
    const target = process.env.API_PROXY_TARGET?.replace(/\/+$/, "");
    if (!target) return [];
    return [{ source: "/api/v1/:path*", destination: `${target}/api/v1/:path*` }];
  },
  turbopack: {
    rules: {
      "*.css": {
        loaders: ["@tailwindcss/turbopack"],
        as: "*.css",
      },
    },
  },
};

export default nextConfig;
