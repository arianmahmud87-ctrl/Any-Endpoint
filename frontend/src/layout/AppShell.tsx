import { useState } from "react";
import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { Activity, Boxes, Gauge, KeyRound, LogOut, Menu, MessageSquare, PanelLeft, Server, Settings } from "lucide-react";
import { cn } from "@/lib/utils";
import { useSession } from "@/auth/session";
import { PREVIEW_MODE } from "@/lib/api/client";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Sheet, SheetContent } from "@/components/ui/sheet";
import { Logo } from "./Logo";

const primary = [
  { to: "/app/playground", label: "Playground", icon: MessageSquare },
  { to: "/app/overview", label: "Overview", icon: Gauge },
  { to: "/app/profiles", label: "Profiles", icon: Boxes },
  { to: "/app/keys", label: "Keys", icon: KeyRound },
  { to: "/app/usage", label: "Usage", icon: Activity },
  { to: "/app/workers", label: "Workers", icon: Server },
  { to: "/app/settings", label: "Settings", icon: Settings },
];

function Item({ to, label, icon: Icon, onNav }: { to: string; label: string; icon: typeof Gauge; onNav?: () => void }) {
  return (
    <NavLink
      to={to}
      onClick={onNav}
      className={({ isActive }) => cn("flex h-8 items-center gap-2.5 rounded-lg px-2 text-[13.5px] transition hover:bg-sidebar-accent", isActive && "bg-sidebar-accent font-medium")}
    >
      <Icon className="h-[15px] w-[15px]" strokeWidth={1.8} />
      <span className="flex-1">{label}</span>
    </NavLink>
  );
}

function SidebarBody({ onCollapse, onNav }: { onCollapse?: () => void; onNav?: () => void }) {
  const { recentRuns } = useSession();
  return (
    <div className="flex h-full flex-col px-3 py-3">
      <div className="mb-5 flex items-center justify-between px-1">
        <Logo />
        {onCollapse && (
          <button onClick={onCollapse} className="rounded-md p-1.5 text-muted-foreground hover:bg-sidebar-accent" aria-label="Collapse sidebar">
            <PanelLeft className="h-4 w-4" strokeWidth={1.7} />
          </button>
        )}
      </div>
      <nav className="space-y-0.5">
        {primary.map((i) => <Item key={i.to} {...i} onNav={onNav} />)}
      </nav>
      <div className="mt-8 px-2 text-[12.5px] text-muted-foreground">Runs</div>
      <div className="mt-2 min-h-0 flex-1 space-y-0.5 overflow-y-auto">
        {recentRuns.length === 0 ? (
          <p className="px-2 text-[12.5px] text-muted-foreground/80">Runs from this session show up here. They aren't saved.</p>
        ) : (
          recentRuns.map((r) => <div key={r.id} className="truncate rounded-lg px-2 py-1.5 text-[13px] text-foreground/80">{r.title}</div>)
        )}
      </div>
      {PREVIEW_MODE && <div className="mx-1 mt-3 rounded-lg border border-dashed px-2.5 py-2 text-[11.5px] text-muted-foreground">Preview mode · demo data</div>}
    </div>
  );
}

function Account() {
  const { me, logout } = useSession();
  const nav = useNavigate();
  const initials = (me?.user.name ?? "?").split(" ").map((s) => s[0]).join("").slice(0, 2);
  return (
    <DropdownMenu>
      <DropdownMenuTrigger aria-label="Account" className="flex h-8 w-8 items-center justify-center rounded-full bg-foreground text-[11px] font-medium text-background transition hover:opacity-85 active:scale-95">
        {initials}
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56 rounded-xl">
        <DropdownMenuLabel className="font-normal">
          <div className="text-[13px] font-medium">{me?.user.name}</div>
          <div className="text-[12px] text-muted-foreground">{me?.user.email}</div>
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={() => nav("/app/settings")}>Settings</DropdownMenuItem>
        <DropdownMenuItem onClick={() => logout()}><LogOut className="mr-2 h-3.5 w-3.5" />Log out</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export default function AppShell() {
  const [collapsed, setCollapsed] = useState<boolean>(false);
  const [mobile, setMobile] = useState<boolean>(false);
  return (
    <div className="flex h-full">
      <aside className={cn("hidden shrink-0 overflow-hidden transition-[width] duration-300 ease-out md:block", collapsed ? "w-0" : "w-[240px]")}>
        <div className="h-full w-[240px]"><SidebarBody onCollapse={() => setCollapsed(true)} /></div>
      </aside>
      <Sheet open={mobile} onOpenChange={setMobile}>
        <SheetContent side="left" className="w-[260px] bg-[hsl(var(--canvas))] p-0">
          <SidebarBody onNav={() => setMobile(false)} />
        </SheetContent>
      </Sheet>
      <main className="relative m-0 flex min-w-0 flex-1 flex-col overflow-hidden bg-background md:my-1.5 md:mr-1.5 md:rounded-2xl md:border">
        <header className="flex h-14 shrink-0 items-center justify-between px-4">
          <div className="flex items-center gap-1">
            <button className="rounded-md p-1.5 text-muted-foreground hover:bg-secondary md:hidden" onClick={() => setMobile(true)} aria-label="Open menu">
              <Menu className="h-4 w-4" />
            </button>
            {collapsed && (
              <button className="hidden rounded-md p-1.5 text-muted-foreground hover:bg-secondary md:block" onClick={() => setCollapsed(false)} aria-label="Expand sidebar">
                <PanelLeft className="h-4 w-4" strokeWidth={1.7} />
              </button>
            )}
          </div>
          <Account />
        </header>
        <div className="min-h-0 flex-1 overflow-y-auto"><Outlet /></div>
      </main>
    </div>
  );
}
