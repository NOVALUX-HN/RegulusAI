// Regulus AI — release hash verification (NOV-70).
//
// Release binaries shipped under binaries/ (and bin/regulus.exe) are
// pinned by checksums.txt next to them. verifyFile() fails closed on a
// mismatch or a missing checksums file when the binary name is listed
// there; binaries with no entry (local `go build` dev outputs) are not
// releases and run untouched. Results are cached per path+mtime+size so
// repeat spawns pay one hash per changed binary, not per invocation.

const { createHash } = require("node:crypto");
const fs = require("node:fs");
const path = require("node:path");

const cache = new Map();

function cacheKey(p) {
  try {
    const st = fs.statSync(p);
    return `${p}|${st.mtimeMs}|${st.size}`;
  } catch {
    return null;
  }
}

function loadChecksums(dir) {
  const file = path.join(dir, "checksums.txt");
  let body;
  try {
    body = fs.readFileSync(file, "utf8");
  } catch {
    return null;
  }
  const out = new Map();
  for (const line of body.split("\n")) {
    const m = line.trim().match(/^([0-9a-fA-F]{64})\s+(\S+)$/);
    if (m) out.set(m[2], m[1].toLowerCase());
  }
  return out;
}

function sha256File(p) {
  const h = createHash("sha256");
  const fd = fs.openSync(p, "r");
  try {
    const buf = Buffer.alloc(1024 * 1024);
    let n;
    while ((n = fs.readSync(fd, buf, 0, buf.length, null)) > 0) {
      h.update(buf.subarray(0, n));
    }
  } finally {
    fs.closeSync(fd);
  }
  return h.digest("hex");
}

// verifyFile returns {ok:true} or {ok:false, reason}. A null entry map
// (no checksums.txt) or an unlisted basename means "not a release":
// allowed to run, since dev builds carry no pin.
function verifyFile(binPath, searchDirs) {
  const base = path.basename(binPath);
  const key = cacheKey(binPath);
  if (key && cache.has(key)) return cache.get(key);
  let result = { ok: true, cached: false, pinned: false };
  for (const dir of searchDirs) {
    const sums = loadChecksums(dir);
    if (!sums || !sums.has(base)) continue;
    result = { ok: true, cached: false, pinned: true };
    let actual;
    try {
      actual = sha256File(binPath);
    } catch (e) {
      result = { ok: false, reason: `cannot hash ${binPath}: ${e.message}`, pinned: true };
      break;
    }
    if (actual !== sums.get(base)) {
      result = {
        ok: false,
        pinned: true,
        reason: `hash mismatch for ${base}: got ${actual}, want ${sums.get(base)} (refusing to run a tampered release binary)`,
      };
    }
    break;
  }
  if (key) {
    result.cached = cache.has(key);
    cache.set(key, result);
  }
  return result;
}

module.exports = { verifyFile, loadChecksums, sha256File };
