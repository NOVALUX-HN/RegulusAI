// NOV-70: release hash verification. Tampered release binaries fail
// closed; dev builds without a pin run untouched.
import { test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import crypto from "node:crypto";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);
const { verifyFile, sha256File } = require("./verify-hash.js");

function tmpFile(dir, name, body) {
  const p = path.join(dir, name);
  fs.writeFileSync(p, body);
  return p;
}

test("pinned binary with matching hash runs", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "vhash-"));
  const bin = tmpFile(dir, "regulus-linux-amd64", "fake-binary-bytes");
  const sum = crypto.createHash("sha256").update("fake-binary-bytes").digest("hex");
  fs.writeFileSync(path.join(dir, "checksums.txt"), `${sum}  regulus-linux-amd64\n`);
  const r = verifyFile(bin, [dir]);
  assert.equal(r.ok, true);
  assert.equal(r.pinned, true);
});

test("tampered release binary fails closed", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "vhash-"));
  const bin = tmpFile(dir, "regulus-windows-amd64.exe", "original-bytes");
  const sum = crypto.createHash("sha256").update("original-bytes").digest("hex");
  fs.writeFileSync(path.join(dir, "checksums.txt"), `${sum}  regulus-windows-amd64.exe\n`);
  fs.writeFileSync(bin, "tampered-bytes");
  const r = verifyFile(bin, [dir]);
  assert.equal(r.ok, false);
  assert.match(r.reason, /hash mismatch/);
});

test("dev binary without pin entry runs untouched", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "vhash-"));
  const bin = tmpFile(dir, "regulus", "dev-bytes");
  fs.writeFileSync(path.join(dir, "checksums.txt"), `${"0".repeat(64)}  other-binary\n`);
  const r = verifyFile(bin, [dir]);
  assert.equal(r.ok, true);
  assert.equal(r.pinned, false);
});

test("missing checksums file means not a release", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "vhash-"));
  const bin = tmpFile(dir, "regulus", "x");
  const r = verifyFile(bin, [dir]);
  assert.equal(r.ok, true);
  assert.equal(r.pinned, false);
});

test("sha256File matches createHash", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "vhash-"));
  const p = tmpFile(dir, "f.bin", "abc123");
  assert.equal(sha256File(p), crypto.createHash("sha256").update("abc123").digest("hex"));
});
