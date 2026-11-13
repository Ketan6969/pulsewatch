import "@fontsource-variable/inter";
import "@fontsource-variable/jetbrains-mono";
import "./styles/tokens.css";
import "./styles/base.css";
import "./styles/layout.css";
import "./styles/components.css";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { createBrowserRouter, RouterProvider } from "react-router-dom";
import { ApiError } from "./api/client";
import { AppShell } from "./layout/AppShell";
import { ThemeProvider } from "./lib/theme";
import { TimeRangeProvider } from "./lib/timeRange";
import { AlertsPage } from "./pages/AlertsPage";
import { HostDetailPage } from "./pages/HostDetailPage";
import { HostsPage } from "./pages/HostsPage";
import { InfrastructurePage } from "./pages/InfrastructurePage";
import { MetricsPage } from "./pages/MetricsPage";
import { MonitorDetailPage } from "./pages/MonitorDetailPage";
import { MonitorsPage } from "./pages/MonitorsPage";
import { NotFoundPage } from "./pages/NotFoundPage";
import { OverviewPage } from "./pages/OverviewPage";
import { SettingsPage } from "./pages/SettingsPage";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 5_000,
      refetchOnWindowFocus: true,
      // Don't retry client errors (404, 422, 401); do retry transient failures.
      retry: (count, err) => !(err instanceof ApiError && err.status >= 400 && err.status < 500) && count < 2,
    },
  },
});

const router = createBrowserRouter([
  {
    element: <AppShell />,
    children: [
      { path: "/", element: <OverviewPage /> },
      { path: "/infrastructure", element: <InfrastructurePage /> },
      { path: "/hosts", element: <HostsPage /> },
      { path: "/hosts/:id", element: <HostDetailPage /> },
      { path: "/monitors", element: <MonitorsPage /> },
      { path: "/monitors/:id", element: <MonitorDetailPage /> },
      { path: "/alerts", element: <AlertsPage /> },
      { path: "/metrics", element: <MetricsPage /> },
      { path: "/settings", element: <SettingsPage /> },
      { path: "*", element: <NotFoundPage /> },
    ],
  },
]);

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ThemeProvider>
      <QueryClientProvider client={queryClient}>
        <TimeRangeProvider>
          <RouterProvider router={router} />
        </TimeRangeProvider>
      </QueryClientProvider>
    </ThemeProvider>
  </StrictMode>,
);
