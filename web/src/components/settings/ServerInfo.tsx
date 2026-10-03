import { useStatus } from "../../stores/status";

/** How the server encodes, and how it's configured. */
export default function ServerInfo() {
  const status = useStatus((s) => s.status);
  return (
    <>
      <section className="card p-4">
        <div className="card-title mb-3">Transcoding</div>
        <dl className="kv-grid text-sm">
          <dt className="text-content-muted">Encoder</dt>
          <dd>
            {status?.transcode ? (status.transcode.hwaccel === "none" ? "Software (CPU)" : `Hardware: ${status.transcode.hwaccel.toUpperCase()}`) : "–"}
            {status?.transcode && status.transcode.requested !== "none" && status.transcode.hwaccel === "none" && (
              <span className="ml-2 text-xs text-warning">
                {status.transcode.requested.toUpperCase()} was requested but isn't usable; check the server log
              </span>
            )}
          </dd>
          {status?.transcode?.hwaccel === "vaapi" && (
            <>
              <dt className="text-content-muted">GPU decoding</dt>
              <dd>
                {status.transcode.gpuDecode
                  ? "Yes: decoding and scaling run on the GPU too"
                  : "No: the CPU decodes and the GPU only encodes (check the server log for the vaapi decode test)"}
              </dd>
            </>
          )}
          <dt className="text-content-muted">HDR tone mapping</dt>
          <dd>
            {status?.transcode
              ? status.transcode.gpuTonemap
                ? "On the GPU"
                : status.transcode.tonemap
                  ? "On the CPU"
                  : "Unavailable (HDR will look washed out)"
              : "–"}
          </dd>
          <dt className="text-content-muted">Live streams</dt>
          <dd>{status?.transcode ? `${status.transcode.active} of ${status.transcode.maxSessions}` : "–"}</dd>
          <dt className="text-content-muted">Optimized copies</dt>
          <dd>{status?.transcode ? `Up to ${status.transcode.optimizeHeight}p, ${status.transcode.encodeWorkers} at a time` : "–"}</dd>
        </dl>
        <p className="mt-3 text-xs text-content-muted">
          Set <span className="mono">COUCHSIDE_HWACCEL=vaapi</span> (Intel/AMD, needs <span className="mono">/dev/dri</span>), <span className="mono">qsv</span> or{" "}
          <span className="mono">nvenc</span> on the server to use a GPU. Unusable settings fall back to software automatically.
        </p>
      </section>

      <section className="card p-4">
        <div className="card-title mb-3">Server</div>
        <dl className="kv-grid text-sm">
          <dt className="text-content-muted">Version</dt>
          <dd className="mono">{status?.version ?? "–"}</dd>
          <dt className="text-content-muted">Media root</dt>
          <dd className="mono">{status?.mediaRoot || "Not restricted"}</dd>
          <dt className="text-content-muted">Automatic rescan</dt>
          <dd>{status?.scanEvery === "0s" ? "Off" : `Every ${status?.scanEvery ?? "–"}`}</dd>
          <dt className="text-content-muted">Background workers</dt>
          <dd>{status?.workers ?? "–"}</dd>
        </dl>
      </section>
    </>
  );
}
