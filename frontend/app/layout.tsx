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

export const metadata: Metadata = {
  title: "Portfolio Manager",
  description: "B3 import, holdings review, and portfolio analysis.",
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
          <div className="pb-20 md:pb-0">{children}</div>
          <BottomNav />
        </VisibilityProvider>
      </body>
    </html>
  );
}
