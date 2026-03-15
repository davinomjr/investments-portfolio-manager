import type { Metadata } from "next";
import { TopNav } from "@/components/top-nav";
import { VisibilityProvider } from "@/components/visibility-context";
import "./globals.css";

export const metadata: Metadata = {
  title: "Portfolio Manager",
  description: "B3 import, holdings review, and portfolio analysis.",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en">
      <body>
        <VisibilityProvider>
          <TopNav />
          {children}
        </VisibilityProvider>
      </body>
    </html>
  );
}
