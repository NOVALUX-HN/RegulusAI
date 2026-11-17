# F14.8 — Revisión independiente de evidencias y experimento decisivo

Revisor: misma sesión, segunda pasada adversarial sobre F14.1–F14.7 + artefactos.

## 1. Matriz afirmación → evidencia → reproducción → confianza

| Afirmación | Evidencia primaria | Reproducción | Confianza |
|---|---|---|---|
| El fallo es en carga, no en compilación | compile exit 0 + 3077 (T2, T3, T10-re) | 4 builds + bloques independientes | alta |
| Mecanismo = SAC VerifiedAndReputable `{0283ac0f…}` | PolicyName/GUID/Hash en 3077 + `.cip` activa + docs MS | GUID citado en Q&A oficial | alta |
| Firma no discrimina | ambos `.test.exe` NotSigned, uno corre y otro no (T2) | — (hecho estático) | alta |
| Ruta/caché/procedencia/MOTW no discriminan | mismos bytes 3 rutas bloqueados (T9) | 3/3 + CTRL Temp | alta |
| Creador no discrimina | copia PS bloqueada igual (T7 CTRL2) | 1/1 | media |
| Lanzador sí discrimina | 15/15 shell bloquea, 6/6 go-gestionado corre (T7) | matriz repetida | media-alta |
| go-gestionado no-determinístico entre días | fallos previos CLI/mbench/harness vs corridas de hoy | historial + reintento | media |
| Reputación por hash (no por archivo) | **copia 5B66 con nombre/ruta/USN nuevos corre** (decisivo T10) | 1/1, hash verificado | media-alta |
| Defender/AppLocker no participan | sin eventos de bloqueo propios (T2) | 2 ventanas | media-alta |
| Enforcement (no audit) | denegaciones efectivas 3077 Error | cada repro | alta |

## 2. Hipótesis descartadas / confirmadas / abiertas

- Descartadas: firma como discriminador; ruta/cache/MOTW/procedencia; creador del
  archivo; per-file-first-seen (la copia 5B66 con identidad de archivo nueva corre);
  Defender o AppLocker como causa; fallo de compilación o permisos.
- Confirmadas: SAC-VerifiedAndReputable deniega en carga; bytes idénticos =
  mismo destino ante mismo lanzador; shell-directo + hash desconocido = denegación.
- Abiertas: mecanismo interno por el que go-gestionado evita el 3077; umbral de
  reputación; rol de las demás `.cip`; por qué `bin/regulus.exe` (firma local)
  corre — candidatas: reputación acumulada o confianza distinta, sin evidencia.

## 3. Falsos positivos y muestras no comparables revisados

- Defender 1150/1151/2010: rutina, no atribuidos a bloqueos. AppLocker 8001:
  policy-applied del 15/11, sin denegaciones. Ningún evento fuera de ventana fue
  usado como causa (T2 correlacionó por timestamp + ruta + hash).
- Tests "corridos" vía `go test`: pasaron de verdad (assertions ejecutadas, 0.2s,
  no cached) — no son falsos positivos. `go vet`/`build`/`dry-run` no cuentan
  como ejecución (declarado en T3).
- N pequeño (3 reps): suficiente para asimetría 9/9 vs 6/6, insuficiente para
  tasas. MOTW: no medible en este FS (unknown, no asumido).

## 4. Experimento decisivo y resultado

Pregunta: ¿la confianza viaja con los bytes o con el archivo? Copia de
`govern.test.exe` (mismos bytes `5B66…`, nombre/ruta/USN/tiempos nuevos) ejecutada
vía bash → **corre**. Conclusión: la reputación es por contenido, no por
identidad de archivo. Refuerza el modelo hash-reputación y elimina el tracking
por archivo como explicación.

## 5. Recomendación final para F15 (NOV-115: sin invalidación)

La evidencia nueva **refuerza** la recomendación A (runner in-repo determinista +
clasificación) y no invalida nada de NOV-115: el runner no necesita hacer
confiable el binario (imposible desde el repo), sino compilar→ejecutar→clasificar
con evidencia por paquete. Se añade un matiz: incluir en F15.1 el caso
"hash reputado corre en cualquier ruta" como prueba de que la clasificación es
el componente determinista, no la ejecución. Fallback (host limpio) y descarte
de falsas universales se mantienen. F15 no parte de causa especulativa: la causa
(SAC VerifiedAndReputable en carga) está probada por eventos + 4 controles.
