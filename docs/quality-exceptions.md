# Quality exceptions (CRAP/audit waivers)

Format (one per line): `file:func motivo expira:YYYY-MM-DD aprueba:nombre`
Example: `# internal/gates/run.go:execAndReport heredado expira:2027-03-01 aprueba:tech-lead`
Blank lines and `#` comments are ignored; the key is only `file:func`.
Discount rule: a waived function is listed (tier `waived`) but discounts
every Max — effective, critical and standard maxima skip it, so it never
fails the gate. Past `expira` means remove the line; waivers never hide
unmeasured code (it stays `unknown`, never 0).
