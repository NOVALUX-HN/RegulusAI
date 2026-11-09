#!/usr/bin/env node
// Regulus AI — platform launcher.
//
// Resolves the regulus binary in the same first-match-wins order as the
// OpenCode plugin (.opencode/plugins/regulus.ts candidates()):
//   1. $REGULUS_BIN
//   2. <root>/bin/regulus.exe
//   3. <root>/bin/regulus
//   4. <root>/binaries/regulus-<platform>-<arch>[.exe]
//   5. ~/.regulus/bin/regulus[.exe]
//   6. `regulus` on PATH
// then execs it, forwarding arguments and stdio untouched. No logic beyond
// resolution lives here: the compiled core owns everything.

const { spawnSync } = require("node:child_process")
const fs = require("node:fs")
const path = require("node:path")

function isRunnable(p) {
  if (typeof p !== "string" || p === "") return false
  try {
    if (!fs.statSync(p).isFile()) return false
  } catch {
    return false
  }
  try {
    fs.accessSync(p, fs.constants.X_OK)
    return true
  } catch {
    // X_OK is existence-equivalent on Windows; on POSIX a failure here
    // means the file is present but not executable.
    return process.platform === "win32"
  }
}

function sameFile(a, b) {
  try {
    return fs.realpathSync(a) === fs.realpathSync(b)
  } catch {
    try {
      return path.resolve(a) === path.resolve(b)
    } catch {
      return false
    }
  }
}

function findOnPath() {
  const dirs = (process.env.PATH || "").split(path.delimiter).filter((d) => d !== "")
  const names = process.platform === "win32" ? ["regulus.exe", "regulus"] : ["regulus"]
  for (const dir of dirs) {
    for (const name of names) {
      const p = path.join(dir, name)
      if (isRunnable(p) && !sameFile(p, __filename)) return p
    }
  }
  return null
}

function resolveBinary() {
  const root = path.join(__dirname, "..")
  const home = process.env.HOME || process.env.USERPROFILE || ""
  const platform = process.platform
  const arch = process.arch === "x64" ? "amd64" : process.arch
  const ext = platform === "win32" ? ".exe" : ""
  // Same derivation as the plugin: regulus-<node platform>-<arch>[.exe].
  // The shipped binaries/ dir uses the Go OS name ("windows" instead of
  // Node's "win32"), so on Windows the Go spelling is probed as part of
  // the same slot — order is unchanged, the slot just knows both names.
  const binariesPrimary = path.join(root, "binaries", `regulus-${platform}-${arch}${ext}`)
  const binariesAlias =
    platform === "win32" ? path.join(root, "binaries", `regulus-windows-${arch}.exe`) : null
  const homeBin = home ? path.join(home, ".regulus", "bin", `regulus${ext}`) : ""

  const tried = [
    process.env.REGULUS_BIN || "",
    path.join(root, "bin", "regulus.exe"),
    path.join(root, "bin", "regulus"),
    binariesPrimary,
    homeBin,
    "regulus",
  ]

  const envBin = process.env.REGULUS_BIN
  if (envBin && isRunnable(envBin) && !sameFile(envBin, __filename)) return { bin: envBin, tried }

  for (const p of [tried[1], tried[2], binariesPrimary]) {
    if (isRunnable(p) && !sameFile(p, __filename)) return { bin: p, tried }
  }
  if (binariesAlias && isRunnable(binariesAlias) && !sameFile(binariesAlias, __filename)) {
    return { bin: binariesAlias, tried }
  }
  if (homeBin && isRunnable(homeBin) && !sameFile(homeBin, __filename)) {
    return { bin: homeBin, tried }
  }
  const onPath = findOnPath()
  if (onPath) return { bin: onPath, tried }
  return { bin: null, tried }
}

const { bin, tried } = resolveBinary()
if (!bin) {
  const lines = tried.map((t, i) => `  ${i + 1}. ${t === "" ? "(unset)" : t}`)
  console.error(
    `regulus: no working regulus binary found. Tried in plugin order:\n${lines.join("\n")}\n` +
      "Build one with `npm run build` (bash scripts/build.bash) and file an issue with your platform.",
  )
  process.exit(1)
}

const result = spawnSync(bin, process.argv.slice(2), {
  stdio: "inherit",
  env: process.env,
  // windowsHide: the launcher is spawned by GUI hosts (and the
  // opencode background server) that own no console on Windows —
  // without it every regulus invocation flashes a terminal open
  // and shut. Harmless when a console already exists.
  windowsHide: true,
})
if (result.error) {
  // The binary never ran: surface why instead of a silent exit 1.
  // On Windows an OS-level Device Guard / App Control (WDAC) denial
  // arrives here as EUNKNOWN/uv_spawn — never as ENOENT — so name it
  // and point at the confirming steps, not at PATH ghosts.
  const msg = result.error.message || String(result.error)
  const code = typeof result.error.code === "string" ? result.error.code : ""
  const errno = typeof result.error.errno === "number" ? result.error.errno : undefined
  console.error(`regulus: failed to start ${bin}: ${msg}`)
  // Exact codes/errno first: a genuine missing file is ENOENT/−4058
  // and must not read as an OS block. Text covers the libuv rendering
  // (Bun's "EUNKNOWN: unknown error, uv_spawn").
  const blocked =
    code === "EUNKNOWN" ||
    code === "UNKNOWN" ||
    errno === -4094 ||
    /\bEUNKNOWN\b|unknown error|-4094/i.test(`${code} ${msg}`)
  if (blocked) {
    console.error(
      "regulus: on Windows this is the Device Guard / App Control (WDAC) block, not a PATH problem.\n" +
        `regulus: confirm with cmd.exe /C '"${bin}" doctor' (Device Guard prints its organization banner),\n` +
        "regulus: or Event Viewer > Microsoft-Windows-CodeIntegrity/Operational, events 3033/3077.\n" +
        "regulus: outs: ask IT for a hash/signer exception, or run inside WSL — see docs/troubleshooting.md (EUNKNOWN uv_spawn).",
    )
  }
  process.exit(1)
}
process.exit(result.status === null ? 1 : result.status)
