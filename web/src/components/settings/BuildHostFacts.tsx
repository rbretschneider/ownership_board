import type { BuildHost } from "../../api/types";
import { buildDiskFree, factBoolean, formatBytes, OS_LABELS } from "../../lib/buildHosts";

/** The hardware and toolchain facts a host reported about itself. */
export function BuildHostFacts({ host }: { host: BuildHost }) {
  const f = host.facts ?? {};
  const disk = buildDiskFree(host);
  const rows: [string, string][] = [
    ["OS", `${OS_LABELS[host.os]} · ${f.os_version || "Unknown version"} · ${f.arch || host.arch}`],
    ["CPU", `${f.cpu_model || "Unknown"} · ${f.cores ?? "Unknown"} cores / ${f.threads ?? "Unknown"} threads`],
    ["RAM", formatBytes(f.ram_bytes)],
    [
      "Build disk free",
      `${formatBytes(disk)}${host.free_disk_bytes == null && disk !== undefined ? " (reported build volume)" : ""}`,
    ],
    [
      "GPU / driver / OpenGL",
      f.gpus === undefined
        ? "Unknown"
        : f.gpus.length === 0
          ? "None reported"
          : f.gpus
              .map((g) => `${g.name} · ${g.driver || "Unknown driver"} · OpenGL ${g.opengl_version || "Unknown"}`)
              .join("; "),
    ],
    [
      "Toolchains",
      f.toolchains === undefined
        ? "Unknown"
        : Object.keys(f.toolchains).length === 0
          ? "None reported"
          : Object.entries(f.toolchains)
              .map(([name, version]) => `${name}: ${version}`)
              .join("; "),
    ],
    ["Interactive session", factBoolean(f.interactive_session)],
    ["Administrator", factBoolean(f.build_user_is_admin)],
  ];
  return (
    <div className="build-host__facts">
      <p className="muted small">Self-reported by this host.</p>
      <dl className="build-host-facts">
        {rows.map(([label, value]) => (
          <div key={label}>
            <dt>{label}</dt>
            <dd>{value}</dd>
          </div>
        ))}
      </dl>
    </div>
  );
}
