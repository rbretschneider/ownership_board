import { useEffect, type ReactNode } from "react";
import { AppNav, type AppPage } from "./AppNav";
import { BrandMark } from "./BrandMark";

/** Props for {@link PageShell}. */
export interface PageShellProps {
  page: AppPage;
  title: string;
  /** Controls shown beside the title, such as a period picker. */
  actions?: ReactNode;
  /** Use the full window width instead of the reading column. */
  isWide?: boolean;
  children: ReactNode;
}

/** A full page off the board: brand, page links, title, then content. */
export function PageShell({ page, title, actions, isWide = false, children }: PageShellProps) {
  // A page opened from another keeps the window's scroll; start each at the top.
  useEffect(() => {
    window.scrollTo(0, 0);
  }, [page]);
  return (
    <div className="page">
      <header className="topbar">
        <a href="#/" className="brand" aria-label="ownership board home">
          <BrandMark />
          <span className="brand__name">ownership</span>
        </a>
        <span className="topbar__spacer" />
        <AppNav current={page} />
      </header>
      <main className={`page__main${isWide ? " page__main--wide" : ""}`}>
        <div className="page__heading">
          <h1 className="page__title">{title}</h1>
          {actions}
        </div>
        {children}
      </main>
    </div>
  );
}
