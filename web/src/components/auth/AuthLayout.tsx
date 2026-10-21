import type { ReactNode } from "react";
import { BrandMark } from "../common/BrandMark";

/** The centered card used by the sign-in and setup screens. */
export function AuthLayout({ title, subtitle, children }: { title: string; subtitle: string; children: ReactNode }) {
  return (
    <main className="auth">
      <div className="auth__card">
        <div className="auth__brand">
          <BrandMark size={34} />
          <span className="brand__name">ownership</span>
        </div>
        <h1 className="auth__title">{title}</h1>
        <p className="auth__subtitle">{subtitle}</p>
        {children}
      </div>
    </main>
  );
}
