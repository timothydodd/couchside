import { useState } from "react";
import Link from "./Link";
import { Row } from "./Rows";
import { personPhotoUrl } from "../lib/api";
import type { CreditRow } from "../lib/types";

/** A person's photo, or their initial on the placeholder gradient. */
export function PersonPhoto({ person, className = "" }: { person: { personId?: number; id?: number; name: string; hasPhoto: boolean }; className?: string }) {
  const [failed, setFailed] = useState(false);
  const id = person.personId ?? person.id ?? 0;
  return (
    <div className={`person-photo ${className}`}>
      {person.hasPhoto && !failed ? (
        <img src={personPhotoUrl(id)} alt="" loading="lazy" decoding="async" onError={() => setFailed(true)} className="absolute inset-0 h-full w-full object-cover" />
      ) : (
        <div aria-hidden="true" className="poster-placeholder absolute inset-0 flex items-center justify-center text-3xl font-semibold text-content-secondary">
          {Array.from(person.name.trim())[0] ?? "?"}
        </div>
      )}
    </div>
  );
}

/** The billed cast as a row of photos. Each opens what else of theirs is in the library. */
export function CastRow({ cast }: { cast: CreditRow[] }) {
  if (!cast.length) return null;
  return (
    <Row title="Cast">
      {cast.map((c) => (
        <Link key={c.personId} to={`/person/${c.personId}`} className="group w-28 shrink-0" aria-label={`${c.name}${c.role ? `, ${c.role}` : ""}`}>
          <PersonPhoto person={c} className="transition-transform group-hover:-translate-y-0.5 group-focus-visible:ring-2 group-focus-visible:ring-accent" />
          <div className="poster-title truncate group-hover:text-accent">{c.name}</div>
          {c.role && <div className="poster-meta truncate">{c.role}</div>}
        </Link>
      ))}
    </Row>
  );
}

/** Key crew as one line: "Director Name · Screenplay Name, Name". */
export function CrewLine({ crew }: { crew: CreditRow[] }) {
  if (!crew.length) return null;
  // Group by job, keeping the server's order (director first).
  const jobs = new Map<string, CreditRow[]>();
  for (const c of crew)
    for (const job of c.role.split(", ").filter(Boolean)) {
      if (!jobs.has(job)) jobs.set(job, []);
      jobs.get(job)!.push(c);
    }
  return (
    <dl className="mt-4 flex max-w-3xl flex-wrap gap-x-6 gap-y-1 text-sm">
      {[...jobs].map(([job, people]) => (
        <div key={job} className="flex gap-2">
          <dt className="text-content-muted">{job}</dt>
          <dd className="text-content-secondary">
            {people.map((p, i) => (
              <span key={p.personId}>
                {i > 0 && ", "}
                <Link to={`/person/${p.personId}`} className="title-link">
                  {p.name}
                </Link>
              </span>
            ))}
          </dd>
        </div>
      ))}
    </dl>
  );
}
