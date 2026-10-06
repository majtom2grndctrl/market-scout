"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { Activity, Briefcase, Telescope, UserRound } from "lucide-react";

import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
} from "@/components/ui/sidebar";
import { paths } from "@/lib/paths";

// Hand-authored, not derived: `paths` builds hrefs and carries no titles.
const navItems = [
  { title: "Status", href: paths.status, icon: Activity },
  { title: "Postings", href: paths.postings, icon: Briefcase },
  { title: "Profile", href: paths.profile, icon: UserRound },
];

export function AppSidebar() {
  const pathname = usePathname();

  return (
    // Borderless on the page surface: the sidebar reads as part of the page,
    // so the content column, not the chrome, is the focus.
    <Sidebar collapsible="icon" className="group-data-[side=left]:border-r-0">
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" render={<Link href={paths.home} />}>
              <div className="flex aspect-square size-8 items-center justify-center rounded-lg bg-sidebar-primary text-sidebar-primary-foreground">
                <Telescope className="size-4" />
              </div>
              <div className="grid flex-1 text-left leading-tight">
                <span className="truncate font-display text-base font-semibold">Market Scout</span>
                <span className="truncate text-sm text-content-muted">
                  Job market intelligence
                </span>
              </div>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupContent>
            <SidebarMenu>
              {navItems.map((item) => (
                <SidebarMenuItem key={item.href}>
                  <SidebarMenuButton
                    isActive={pathname === item.href}
                    tooltip={item.title}
                    render={<Link href={item.href} />}
                  >
                    <item.icon />
                    <span>{item.title}</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>

      <SidebarRail />
    </Sidebar>
  );
}
