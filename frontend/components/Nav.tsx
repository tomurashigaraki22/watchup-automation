"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

const links = [
  { href: "/", label: "Dashboard" },
  { href: "/companies", label: "Companies" },
  { href: "/emails", label: "Emails" },
  { href: "/campaigns", label: "Campaigns" },
  { href: "/search", label: "Search" },
];

export default function Nav({ onLogout }: { onLogout: () => void }) {
  const pathname = usePathname();

  return (
    <nav className="flex w-56 shrink-0 flex-col border-r border-slate-200 bg-white p-4">
      <div className="mb-6 px-2 text-lg font-semibold text-slate-900">WatchUp</div>
      <div className="flex flex-col gap-1">
        {links.map((link) => {
          const active = pathname === link.href;
          return (
            <Link
              key={link.href}
              href={link.href}
              className={`rounded-md px-2 py-1.5 text-sm font-medium ${
                active ? "bg-brand-50 text-brand-700" : "text-slate-600 hover:bg-slate-50"
              }`}
            >
              {link.label}
            </Link>
          );
        })}
      </div>
      <button onClick={onLogout} className="btn-secondary mt-auto">
        Log out
      </button>
    </nav>
  );
}
