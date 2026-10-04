import Link from "../Link";
import ProfileAvatar from "../ProfileAvatar";
import { Segmented } from "../ui";
import { BREAK_MODES, SUBTITLE_LANGS } from "../../lib/prefs";
import { languageName } from "../../lib/tracks";
import { useProfile } from "../../stores/profile";
import { useStatus } from "../../stores/status";

/** Preferences of the current profile. Everything else on this page is server-wide. */
export default function ProfileSettings() {
  const profile = useProfile((s) => s.current);
  const setPrefs = useProfile((s) => s.setPrefs);
  const liveTv = useStatus((s) => s.status?.livetv?.configured);
  if (!profile) return null;
  const p = profile.prefs;
  return (
    <section className="card p-4">
      <div className="mb-4 flex items-center gap-3">
        <ProfileAvatar profile={profile} size={36} />
        <div className="min-w-0 flex-1">
          <div className="card-title">Your settings</div>
          <div className="text-xs text-content-muted">Only for {profile.name}. Other profiles keep their own.</div>
        </div>
        <Link to="/profiles" className="btn-quiet !text-xs">
          Switch profile
        </Link>
      </div>
      <div className="form-grid text-sm">
        <span className="text-content-muted">Next episode</span>
        <label className="flex items-center gap-2 text-content-secondary">
          <input type="checkbox" className="accent-brand" checked={p.autoplayNext !== false} onChange={(e) => setPrefs({ autoplayNext: e.target.checked })} />
          Play the next episode automatically
        </label>
        <span className="text-content-muted">Subtitles</span>
        <div>
          <select className="field" value={p.subtitleLang ?? ""} onChange={(e) => setPrefs({ subtitleLang: e.target.value })} aria-label="Subtitles">
            <option value="">Only forced subtitles (foreign dialogue)</option>
            {SUBTITLE_LANGS.map((l) => (
              <option key={l} value={l}>
                Always on in {languageName(l)}, when available
              </option>
            ))}
          </select>
        </div>
        <span className="text-content-muted">Intros</span>
        <div>
          <Segmented label="Intros" value={p.intros ?? "button"} onChange={(m) => setPrefs({ intros: m })} options={BREAK_MODES.map((m) => ({ id: m.id, label: m.short.replace("Marked only", "Off") }))} />
          <p className="mt-1 text-xs text-content-muted">For episodes and films whose intro is known (from the file's chapters, or marked by an admin).</p>
        </div>
        <span className="text-content-muted">Commercials</span>
        <div>
          <Segmented label="Commercials" value={p.commercials ?? "auto"} onChange={(m) => setPrefs({ commercials: m })} options={BREAK_MODES.map((m) => ({ id: m.id, label: m.short }))} />
          <p className="mt-1 text-xs text-content-muted">{BREAK_MODES.find((m) => m.id === (p.commercials ?? "auto"))!.detail}, in recordings that have been checked for commercials.</p>
        </div>
        {liveTv && (
          <>
            <span className="text-content-muted">Live TV quality</span>
            <div>
              <Segmented
                label="Live TV quality"
                value={p.liveHeight ?? 720}
                onChange={(h) => setPrefs({ liveHeight: h })}
                options={[1080, 720, 480].map((h) => ({ id: h, label: `${h}p` }))}
              />
            </div>
            <span className="text-content-muted">On TVs</span>
            <label className="flex items-center gap-2 text-content-secondary">
              <input
                type="checkbox"
                className="accent-brand"
                checked={p.livePassthrough !== false}
                onChange={(e) => setPrefs({ livePassthrough: e.target.checked })}
              />
              Play channels in their original broadcast format when the TV can (full quality, almost no server work; up to about 19 Mbps)
            </label>
          </>
        )}
      </div>
    </section>
  );
}
