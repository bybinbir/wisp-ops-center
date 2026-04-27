import type { Metadata } from "next";
import "./globals.css";
import { Sidebar } from "@/components/Sidebar";
import { TopBar } from "@/components/TopBar";

export const metadata: Metadata = {
  title: "WISP Ops Center",
  description:
    "WISP operasyon karar platformu — Bugün ağda ne bozuk, kime müdahale etmeliyim, hangi link riskli?"
};

export default function RootLayout({
  children
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="tr">
      <body>
        <div className="app-shell">
          <Sidebar />
          <main className="app-main">
            <TopBar />
            <div className="app-content">{children}</div>
          </main>
        </div>
      </body>
    </html>
  );
}
