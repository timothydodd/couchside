import type { ReactNode } from "react";

/** Full-screen centred layout for sign-in, first-run setup and password changes. */
export default function AuthShell({ title, subtitle, top, children }: { title: string; subtitle?: ReactNode; top?: ReactNode; children: ReactNode }) {
  return (
    <div className="flex h-full flex-col items-center overflow-auto bg-page px-6 py-10">
      {top && <div className="self-start">{top}</div>}
      <div className="my-auto flex w-full max-w-3xl flex-col items-center py-8">
        <img src="/icons/logo-64.png" alt="" className="mb-6 h-10 w-10" />
        <h1 className="text-center text-2xl font-semibold text-content">{title}</h1>
        {subtitle && <p className="mt-1 max-w-md text-center text-sm text-content-muted">{subtitle}</p>}
        {children}
      </div>
    </div>
  );
}
