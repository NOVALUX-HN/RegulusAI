# NOV-23 — Hash de receipts suficiente en local, sin firma (T21)

Decisión: no firmar. Hash detecta alteración; no hay cruce de dominios de confianza.

Qué garantiza / no garantiza:
- Sí: detección de alteración de contenido (`StoredSHA` pin texto guardado).
- No: autoría frente a quien puede reescribir el archivo.

Evidencia:
- `internal/receipts/receipts.go:19,36-` `Capture()->(prefix,sha256)`, `Write()` append-only `O_EXCL`, `Receipt{stdout_sha256,stderr_sha256,stored_sha256,manifest,run_id,schema=1}`.
- `internal/evidence/verify.go:49-,159-,182-,207-,241-` recomputa Capture/StoredSHA, pin versión, link manifest SHA256, quarantine a `corrupt/` sin borrar.
- `internal/verdict/verdict.go:38-` adjudica sin mutar veredicto por evaluaciones.
- Cero `ed25519/hmac/sign`, solo `crypto/sha256`; `docs/adr/T1-03-verify-without-crypto.md` explícito.

Si receipts cruzan máquinas/CI/dominios: proponer esquema + ciclo de claves + tests válido/modificado/sin-firma/clave-desconocida. Hoy: justificado no implementar.
