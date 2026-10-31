# Dashboard Serve (visual siempre vivo)

Header: Página local siempre viva en puerto fijo con lo importante del
harness, lo guardado y cómo aprende el Almanac para ver la mejora de
desarrollos y proyectos, con arranque automático al encender la PC.

## Specs

S1: "quiero tener una pagina siempre en un puerto especial donde poder ver la informacion importante del harness donde ver informacion relevante de lo que guardamos y tambien de almanac para ver como aprnede, que informacion aprende y conocer como esta mejorando mis desarrollos y proyectos."
S2: "procede con al parte visual que se ejecute en cuanto se encienda la pc siempre porque es necesario siempre tener visibilidad"
S3: "analiza como poder hacer esto"

## Tasks

- T1 (S1): serve read-only + JSON /v1/* sobre stores existentes — route: inline — commit: 62ca372
- T2 (S1,S2): UI embebida Harness/Guardado/Aprendizaje con auto-refresh — route: inline — commit: 62ca372
- T3 (S2): arranque automático al encender la PC (Windows) + docs — route: inline — commit: 62ca372
- T4 (S1,S2,S3): verificación total + probe vivo en puerto fijo — route: inline — commit: —

## Log

L3 (T1-T3 probe vivo puerto 18083): `/` HTML con Harness/Guardado/Aprendizaje + `/v1/metrics` 45 tasks 569 receipts + `/v1/almanac/summary` 42 obs 2 admitidas fresh + POST 405 envelope compartido. `serve --help` y `--port=abc` exit 2. Commit 62ca372 en develop.

L1 (user original verbatim): "quiero tener una pagina siempre en un puerto especial donde poder ver la informacion importante del harness donde ver informacion relevante de lo que guardamos y tambien de almanac para ver como aprnede, que informacion aprende y conocer como esta mejorando mis desarrollos y proyectos."
L2 (user follow-up verbatim): "procede con al parte visual que se ejecute en cuanto se encienda la pc siempre porque es necesario siempre tener visibilidad"
