import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "standalone",
  basePath: process.env.NEXT_PUBLIC_BASE_PATH ?? "",
  experimental: {
    typedRoutes: true,
  },
};

export default nextConfig;
