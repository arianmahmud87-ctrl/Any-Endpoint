import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { ApiError } from "@/lib/api/types";
import { SessionProvider } from "@/auth/session";
import { AuthCallback, AuthGuard, LoginPage } from "@/auth/pages";
import AppShell from "@/layout/AppShell";
import Playground from "@/features/Playground";
import Overview from "@/features/Overview";
import Profiles, { ProfileDetail } from "@/features/Profiles";
import Keys from "@/features/Keys";
import { SettingsPage, Usage, Workers } from "@/features/Ops";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: (n, e) => !(e instanceof ApiError && e.status < 500) && n < 2,
    },
  },
});

const App = () => (
  <QueryClientProvider client={queryClient}>
    <SessionProvider>
      <TooltipProvider>
        <Toaster position="bottom-right" />
        <BrowserRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
          <Routes>
            <Route path="/login" element={<LoginPage />} />
            <Route path="/auth/callback" element={<AuthCallback />} />
            <Route path="/app" element={<AuthGuard><AppShell /></AuthGuard>}>
              <Route index element={<Navigate to="playground" replace />} />
              <Route path="playground" element={<Playground />} />
              <Route path="overview" element={<Overview />} />
              <Route path="profiles" element={<Profiles />} />
              <Route path="profiles/:id" element={<ProfileDetail />} />
              <Route path="keys" element={<Keys />} />
              <Route path="usage" element={<Usage />} />
              <Route path="workers" element={<Workers />} />
              <Route path="settings" element={<SettingsPage />} />
            </Route>
            <Route path="*" element={<Navigate to="/app/playground" replace />} />
          </Routes>
        </BrowserRouter>
      </TooltipProvider>
    </SessionProvider>
  </QueryClientProvider>
);

export default App;
