import { Link } from "react-router-dom";
import { EmptyState } from "../components/ui/states";
import { Page, usePageMeta } from "../layout/AppShell";

export function NotFoundPage() {
  usePageMeta({ title: "Not found", noTimeRange: true });
  return (
    <Page>
      <div className="panel">
        <EmptyState title="Page not found" action={<Link to="/">Back to overview</Link>}>
          The page you requested does not exist.
        </EmptyState>
      </div>
    </Page>
  );
}
