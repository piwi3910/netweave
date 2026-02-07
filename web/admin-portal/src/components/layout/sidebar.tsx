"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  LayoutDashboard,
  Building2,
  Users,
  ShieldCheck,
  FileKey,
  ScrollText,
  Cloud,
  Package,
  Link2,
} from "lucide-react";
import { cn } from "@/lib/utils";

const mainNavigation = [
  { name: "Dashboard", href: "/dashboard", icon: LayoutDashboard },
  { name: "Tenants", href: "/tenants", icon: Building2 },
  { name: "Users", href: "/users", icon: Users },
  { name: "Roles", href: "/roles", icon: ShieldCheck },
  { name: "Certificates", href: "/certificates", icon: FileKey },
  { name: "Audit Log", href: "/audit", icon: ScrollText },
];

const infrastructureNavigation = [
  { name: "O-Clouds", href: "/infrastructure/o-clouds", icon: Cloud },
  { name: "DMS", href: "/infrastructure/dms", icon: Package },
  { name: "Links", href: "/infrastructure/links", icon: Link2 },
];

export function Sidebar() {
  const pathname = usePathname();

  return (
    <aside className="hidden w-64 shrink-0 border-r bg-card lg:block">
      <div className="flex h-16 items-center border-b px-6">
        <Link href="/dashboard" className="flex items-center gap-2">
          <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-primary text-primary-foreground font-bold text-sm">
            NW
          </div>
          <span className="text-lg font-semibold">NetWeave</span>
        </Link>
      </div>
      <nav className="flex flex-col gap-1 p-4">
        {mainNavigation.map((item) => {
          const isActive = pathname.startsWith(item.href);
          return (
            <Link
              key={item.name}
              href={item.href}
              className={cn(
                "flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors",
                isActive
                  ? "bg-primary text-primary-foreground"
                  : "text-muted-foreground hover:bg-accent hover:text-accent-foreground"
              )}
            >
              <item.icon className="h-4 w-4" />
              {item.name}
            </Link>
          );
        })}
        <div className="my-2 border-t" />
        <p className="px-3 py-2 text-xs font-semibold text-muted-foreground uppercase">
          Infrastructure
        </p>
        {infrastructureNavigation.map((item) => {
          const isActive = pathname.startsWith(item.href);
          return (
            <Link
              key={item.name}
              href={item.href}
              className={cn(
                "flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors",
                isActive
                  ? "bg-primary text-primary-foreground"
                  : "text-muted-foreground hover:bg-accent hover:text-accent-foreground"
              )}
            >
              <item.icon className="h-4 w-4" />
              {item.name}
            </Link>
          );
        })}
      </nav>
    </aside>
  );
}
