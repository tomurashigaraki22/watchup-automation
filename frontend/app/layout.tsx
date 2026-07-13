"use client";

import "./globals.css";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { getToken, clearToken } from "@/lib/auth";
import Nav from "@/components/Nav";

export default function RootLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const [ready, setReady] = useState(false);
  const isLogin = pathname === "/login";

  useEffect(() => {
    document.title = "WatchUp Outreach";
    if (!isLogin && !getToken()) {
      router.replace("/login");
      return;
    }
    setReady(true);
  }, [pathname, isLogin, router]);

  return (
    <html lang="en">
      <body className="min-h-screen bg-slate-50 text-slate-900">
        {isLogin ? (
          children
        ) : ready ? (
          <div className="flex min-h-screen">
            <Nav
              onLogout={() => {
                clearToken();
                router.replace("/login");
              }}
            />
            <main className="flex-1 overflow-x-auto p-6">{children}</main>
          </div>
        ) : (
          <div className="flex min-h-screen items-center justify-center text-slate-400">Loading…</div>
        )}
      </body>
    </html>
  );
}
