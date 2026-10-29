---
description: Toggle Regulus YOLO auto-approve on or off without leaving opencode
---

Run `regulus yolo` with the invocation arguments ($ARGUMENTS): `on`, `off`, or `status` (default `status` when no arguments were given).

- Execute it via bash and report the exact output.
- The switch only flips the safe catch-all; the destructive denies (deletes, hard resets, force pushes) always stay denied.
- After `on` or `off`, tell the user to run `/reload` in opencode (or restart) for the new mode to apply — config is read at startup.
