import type { BuildHostEvent, BuildHostOS } from "../api/types";
export const OS_LABELS: Record<BuildHostOS, string> = { windows: "Windows", linux: "Linux", macos: "macOS" };
export const shellLiteral = (value: string) => "'" + value.replaceAll("'", "'\"'\"'") + "'";
export const powershellLiteral = (value: string) => "'" + value.replaceAll("'", "''") + "'";
export function installCommands(base: string, code: string): { label: string; command: string }[] {
  if (!base) return [];
  const url = base.replace(/\/+$/, "");
  const ps = `$installer = Join-Path ([IO.Path]::GetTempPath()) ([IO.Path]::GetRandomFileName() + '.ps1'); try { Invoke-WebRequest -Uri ${powershellLiteral(url + "/install/build-host.ps1")} -OutFile $installer -ErrorAction Stop; & $installer -BoardUrl ${powershellLiteral(url)} -PairingCode ${powershellLiteral(code)} } finally { Remove-Item -LiteralPath $installer -ErrorAction SilentlyContinue }`;
  const sh = `( installer=$(mktemp) || exit 1; trap 'rm -f "$installer"' EXIT HUP INT TERM; curl -fsSL ${shellLiteral(url + "/install/build-host.sh")} -o "$installer" && sh "$installer" --board-url ${shellLiteral(url)} --pairing-code ${shellLiteral(code)} )`;
  return [
    { label: "Windows (PowerShell)", command: ps },
    { label: "Linux", command: sh },
    { label: "macOS", command: sh },
  ];
}
export function formatBytes(value: number | null | undefined): string {
  if (value == null) return "Unknown";
  if (value === 0) return "0 B";
  const i = Math.min(Math.floor(Math.log(value) / Math.log(1024)), 4);
  return `${(value / 1024 ** i).toFixed(i === 0 ? 0 : 1)} ${["B", "KiB", "MiB", "GiB", "TiB"][i]}`;
}
export const factBoolean = (value: boolean | undefined) => (value === undefined ? "Unknown" : value ? "Yes" : "No");
export function eventDescription(event: BuildHostEvent): string {
  if (event.event_type !== "toolchains_changed") return `Host event: ${event.event_type}`;
  const before = event.data?.before ?? {},
    after = event.data?.after ?? {};
  const changes = [...new Set([...Object.keys(before), ...Object.keys(after)])].sort().flatMap((key) => {
    if (before[key] === after[key]) return [];
    if (!(key in before)) return [`Added ${key} ${after[key]}`];
    if (!(key in after)) return [`Removed ${key} ${before[key]}`];
    return [`Changed ${key}: ${before[key]} → ${after[key]}`];
  });
  return changes.join("; ") || "Toolchains unchanged";
}
