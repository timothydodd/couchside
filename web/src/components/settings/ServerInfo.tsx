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
            {status?.transcode && status.transcode.hwaccel === "none" && status.transcode.note && (
              <span className={`mt-1 block text-xs ${status.transcode.requested === "auto" ? "text-content-muted" : "text-warning"}`}>
                {status.transcode.requested === "auto" ? "No usable GPU" : `${status.transcode.requested.toUpperCase()} isn't usable`}:{" "}
                {status.transcode.note}
              </span>
            )}
          </dd>
          {(status?.transcode?.hwaccel === "vaapi" || status?.transcode?.hwaccel === "nvenc") && (
            <>
              <dt className="text-content-muted">GPU decoding</dt>
              <dd>
                {status.transcode.gpuDecode
                  ? "Yes: decoding and scaling run on the GPU too"
                  : "No: the CPU decodes and the GPU only encodes (check the server log for the GPU decode test)"}
              </dd>
            </>
          )}
          <dt className="text-content-muted">HDR tone mapping</dt>
          <dd>
            {status?.transcode
              ? status.transcode.gpuTonemap
                ? status.transcode.tonemapFilter === "tonemap_opencl"
                  ? "On the GPU (OpenCL)"
                  : "On the GPU"
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
          The server picks a GPU encoder it can use (NVENC, Quick Sync, or VAAPI on Linux with{" "}
          <span className="mono">/dev/dri</span>). Set <span className="mono">COUCHSIDE_HWACCEL</span> to{" "}
          <span className="mono">nvenc</span>, <span className="mono">qsv</span>, <span className="mono">vaapi</span> or{" "}
          <span className="mono">none</span> to choose.
        </p>
      </section>

      <section className="card p-4">
        <div className="card-title mb-3">Server</div>
        <dl className="kv-grid text-sm">
          <dt className="text-content-muted">Version</dt>
          <dd className="mono">{status?.version ?? "–"}</dd>
          <dt className="text-content-muted">Media locations</dt>
          <dd className="mono">{status?.mediaRoots?.length ? status.mediaRoots.map((r) => <div key={r}>{r}</div>) : "None yet"}</dd>
          <dt className="text-content-muted">Automatic rescan</dt>
          <dd>{status?.scanEvery === "0s" ? "Off" : `Every ${status?.scanEvery ?? "–"}`}</dd>
          <dt className="text-content-muted">Background workers</dt>
          <dd>{status?.workers ?? "–"}</dd>
        </dl>
      </section>
    </>
  );
}
