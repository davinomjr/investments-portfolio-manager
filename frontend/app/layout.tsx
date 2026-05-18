import type { Metadata, Viewport } from "next";
import { TopNav } from "@/components/top-nav";
import { BottomNav } from "@/components/bottom-nav";
import { VisibilityProvider } from "@/components/visibility-context";
import "./globals.css";

export const viewport: Viewport = {
  themeColor: "#0f172a",
  width: "device-width",
  initialScale: 1,
};

const basePath = process.env.NEXT_PUBLIC_BASE_PATH ?? "";

export const metadata: Metadata = {
  title: "Portfolio Manager",
  description: "B3 import, holdings review, and portfolio analysis.",
  // Set an explicit icon so the browser doesn't fall back to /favicon.ico
  // at the page origin's root — on davinomjr.com that resolves to an
  // unrelated HTTP URL and triggers a mixed-content block.
  icons: { icon: `${basePath}/icon.svg` },
  appleWebApp: {
    capable: true,
    statusBarStyle: "black-translucent",
    title: "Portfolio",
  },
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en">
      <body>
        <VisibilityProvider>
          <TopNav />
          <div className="pb-32 md:pb-0">{children}</div>
          <BottomNav />
        </VisibilityProvider>
      </body>
    </html>
  );
}
