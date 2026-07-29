# Propuesta: integración Engram–Codex orientada a continuidad

Fecha: 2026-07-29

Estado: propuesta para discusión; no autoriza implementación

Ámbito: perfil `agents` de `dots`, integración Codex de Engram

## Dirección propuesta, pendiente de decisión

`dots` debe declarar **qué política de memoria quiere la workstation** y Engram
debe seguir siendo el único responsable de **renderizar e instalar su
integración Codex**.

La solución objetivo es:

1. añadir upstream a Engram un perfil Codex coherente llamado
   `continuity` (nombre provisional);
2. añadir después un Provisioner allowlisted `engram` a `dots`, que invoque
   exactamente `engram setup codex --profile=continuity`;
3. retirar `engram` de los componentes Codex instalados indirectamente por
   `gentle-ai`, para que exista un solo propietario del setup;
4. no parchear `~/.codex/plugins/cache`, no versionar los archivos completos
   generados por Engram y no convertir `dots` en un runner genérico de hooks.

La recomendación anterior de “eliminar `UserPromptSubmit`” se refina: en el
perfil `continuity` conviene mantener el hook como efecto lateral silencioso
para asociar prompts con observaciones, pero quitarle ToolSearch, `mem_context`
y recordatorios. Eliminarlo entero también elimina captura útil que no consume
contexto del modelo.

## Pregunta

¿Cómo conservar el valor demostrado de Engram —continuidad entre sesiones,
recuperación tras compactación y decisiones duraderas— sin usarlo como
bitácora duplicada de Git, GitHub, tests, bundles de delivery y respuestas de
subagentes?

## Evidencia primaria

La investigación usa el Engram instalado y el commit exacto que el marketplace
Codex tenía activo:

- Engram CLI local: `1.20.0`.
- Codex local: `0.146.0`, con `hooks` y `plugins` estables.
- Marketplace Engram instalado y `main` upstream:
  `763a6ba432713725d6ce82a2416eec6cbd9ec94e`.
- [Instalador Codex de Engram](https://github.com/Gentleman-Programming/engram/blob/763a6ba432713725d6ce82a2416eec6cbd9ec94e/internal/setup/setup.go#L1155-L1267).
- [Registro de agentes de Engram](https://github.com/Gentleman-Programming/engram/blob/763a6ba432713725d6ce82a2416eec6cbd9ec94e/internal/setup/agents.go).
- [Manifiesto de hooks Codex](https://github.com/Gentleman-Programming/engram/blob/763a6ba432713725d6ce82a2416eec6cbd9ec94e/plugin/codex/hooks/hooks.json).
- [SessionStart Codex](https://github.com/Gentleman-Programming/engram/blob/763a6ba432713725d6ce82a2416eec6cbd9ec94e/plugin/codex/scripts/session-start.sh).
- [UserPromptSubmit Codex](https://github.com/Gentleman-Programming/engram/blob/763a6ba432713725d6ce82a2416eec6cbd9ec94e/plugin/codex/scripts/user-prompt-submit.sh).
- [SubagentStop Codex](https://github.com/Gentleman-Programming/engram/blob/763a6ba432713725d6ce82a2416eec6cbd9ec94e/plugin/codex/scripts/subagent-stop.sh).
- [Instrucciones del servidor MCP](https://github.com/Gentleman-Programming/engram/blob/763a6ba432713725d6ce82a2416eec6cbd9ec94e/internal/mcp/mcp.go#L173-L241).
- [Propuesta upstream de `--protocol`](https://github.com/Gentleman-Programming/engram/blob/763a6ba432713725d6ce82a2416eec6cbd9ec94e/openspec/changes/archive/2026-07-08-setup-protocol-flag/proposal.md).
- Contrato local de dots: [`CONTEXT.md`](../../CONTEXT.md),
  [`dots.yaml`](../../dots.yaml) y
  [ADR 0004](../adr/0004-codex-mcp-provisioner.md).

No se ejecutó `engram setup codex` ni se modificó configuración real. Las
comprobaciones sobre `~/.codex` fueron de solo lectura.

## Estado actual

### Qué instala Engram

`installCodex`:

1. escribe `~/.codex/engram-instructions.md`;
2. escribe `~/.codex/engram-compact-prompt.md`;
3. registra `[mcp_servers.engram]` en `config.toml`;
4. apunta `model_instructions_file` y
   `experimental_compact_prompt_file` a esos archivos;
5. instala el marketplace/plugin Codex, que añade MCP, skill y hooks.

La integración reparte una misma política entre varias superficies:

| Superficie | Comportamiento actual | Valor | Coste/ruido |
| --- | --- | --- | --- |
| MCP `serverInstructions` | describe herramientas y exige guardado proactivo | descubrimiento siempre disponible | duplica la política global |
| `model_instructions_file` | instala el protocolo detallado | comportamiento aun sin hooks | se carga en todas las tareas |
| skill `engram-memory` | repite protocolo y se declara `ALWAYS ACTIVE` | referencia detallada | tercera copia potencial |
| `SessionStart` | inicia servidor/sesión, importa sync, imprime protocolo y contexto | lifecycle y continuidad | mezcla efectos laterales con tokens |
| `UserPromptSubmit` | captura prompt; primer turno fuerza ToolSearch/contexto; luego recuerda guardar | trazabilidad de prompts | tool calls y nudges redundantes |
| `SubagentStop` | manda toda respuesta del subagente a captura pasiva | puede extraer hallazgos | principal fuente observada de duplicados |
| `Stop` | termina la sesión por HTTP | lifecycle barato | prácticamente ninguno |
| compact prompt + hook | ambos instruyen recuperación/persistencia | recuperación | instrucciones superpuestas |

Medición local orientativa, sin convertir palabras a tokens:

- `engram-instructions.md`: 7.184 bytes, 1.096 palabras.
- bloque estático observado en `SessionStart`: 2.116 bytes, 307 palabras.
- `serverInstructions` upstream: 2.880 bytes, 391 palabras.
- skill completo upstream: 5.761 bytes, 872 palabras si llega a cargarse.

No deben sumarse mecánicamente: Codex no necesariamente carga todas las
superficies del mismo modo. Sí demuestran que existe duplicación estructural,
no solo una impresión subjetiva.

### `--protocol=slim` todavía no resuelve Codex

`engram setup codex --help` acepta `--protocol=slim|full`, pero informa que
`slim` solo tiene efecto en `claude-code`. La propuesta upstream que introdujo
el flag dejó explícitamente fuera:

- las instrucciones Codex escritas en setup;
- los adapters no-Claude;
- `serverInstructions` del MCP.

Por tanto, declarar hoy `--protocol=slim` desde `dots` sería una falsa
convergencia: el comando saldría correctamente sin cambiar las superficies que
causan el problema.

### Control de `serverInstructions`

El protocolo MCP permite que el servidor devuelva un campo opcional
`instructions` durante `initialize`. El servidor decide su contenido; el
protocolo no define una negociación para que el cliente solicite variantes
como `full`, `slim` o `none`.

En la combinación investigada:

- Codex `0.146.0` permite desactivar el servidor MCP completo con `enabled =
  false`;
- Codex permite seleccionar herramientas con `enabled_tools` y
  `disabled_tools`;
- ninguna opción documentada de Codex permite omitir o sustituir únicamente
  las instrucciones del servidor;
- Engram `1.20.0`, en el commit upstream investigado, define
  `serverInstructions` como una constante y la entrega siempre mediante
  `server.WithInstructions`;
- filtrar herramientas no reduce ese texto;
- `engram setup codex --protocol=slim` tampoco lo modifica.

Por tanto, `dots` no puede reducir `serverInstructions` declarativamente con
las superficies soportadas hoy. Puede apagar todo Engram MCP, pero no conservar
sus herramientas y omitir solo sus instrucciones.

La mejora upstream mínima sería menor que el perfil completo:

```text
engram mcp --instructions=full|slim|none
```

El nombre exacto queda por acordar. También podría ser configuración persistida
o una variable de entorno, siempre que el proceso MCP seleccione la política
antes de construir `serverInstructions`. Este contrato permitiría experimentar
con instrucciones reducidas sin esperar a que Engram implemente simultáneamente
todos los cambios de hooks y setup del perfil `continuity`.

### Límite arquitectónico de dots

El dominio local define un Provisioner como una invocación allowlisted e
idempotente y define los archivos que el tool reescribe como **Regenerated
Content**. `dots` versiona la invocación, no ese contenido.

Modificar el cache del plugin después de cada setup viola ese límite:

- el cache es versionado y reemplazable por Codex/Engram;
- `hooks.json` no ofrece un mecanismo de merge para ser un Config Overlay;
- los hashes de confianza cambian;
- `dots` pasaría a conocer el layout interno y versión del plugin;
- una actualización upstream podría dejar una integración parcialmente activa.

## Política `continuity`

El perfil debe optimizar continuidad, no volumen de memoria:

| Componente | Política |
| --- | --- |
| MCP Engram | mantener |
| MCP `serverInstructions` | versión corta: propósito, herramientas core y búsqueda bajo demanda |
| `model_instructions_file` | omitir si MCP slim cubre la política; nunca repetir el protocolo |
| skill | referencia on-demand, sin `ALWAYS ACTIVE` |
| `SessionStart` normal | mantener servidor, sesión e import; stdout vacío |
| contexto al iniciar | no inyectar; consultar solo ante trabajo previo o falta real de contexto |
| `UserPromptSubmit` | mantener captura del prompt; devolver `{}` |
| recordatorios de guardado | desactivar |
| `SubagentStop` | no-op/desactivar |
| `Stop` | mantener |
| compactación | una sola instrucción breve de recuperación; sin repetir el protocolo |
| captura pasiva por “Key Learnings” | desactivar |
| resumen | uno al cerrar una sesión significativa |

Reglas semánticas:

- guardar decisiones duraderas, preferencias confirmadas, bugs con causa raíz
  y descubrimientos no obvios;
- no guardar commits, tests, PRs, outputs de subagentes ni pasos ya
  representados por una autoridad canónica;
- buscar memoria cuando el usuario menciona trabajo anterior, al reanudar
  trabajo o después de compactación; no por cada primer prompt de proyecto;
- usar `topic_key` para evolución real, no para replicar el mismo evento;
- el bundle de delivery, Git y GitHub siguen siendo autoridad; Engram aporta
  continuidad y conocimiento no derivable.

## Arquitectura objetivo

### Upstream Engram

Añadir un perfil estable y consultable:

```text
engram setup codex --profile=continuity
engram integration-profile codex
```

`full` conserva compatibilidad. `continuity` debe afectar conjuntamente:

- instrucciones MCP;
- archivos de instrucciones/compactación;
- comportamiento runtime de los scripts;
- captura de prompts;
- nudges;
- captura de subagentes.

Los scripts pueden leer el perfil en runtime, como ya hacen los hooks Claude
con `protocol-mode`. Así el manifiesto del plugin puede permanecer intacto:
`SubagentStop` sigue registrado pero hace no-op en `continuity`, y
`UserPromptSubmit` conserva solo el POST del prompt. No hace falta que `dots`
edite el cache.

### dots

Después de que exista el contrato upstream, añadir un dialecto cerrado:

```yaml
provisioners:
  - tool: engram
    tags: [agents]
    os: [darwin, linux]
    spec:
      agent: codex
      profile: continuity
    dependencies:
      - name: engram
        command: engram
```

El renderer solo aceptaría agentes/perfiles conocidos y produciría un argv
exacto. No aceptaría hooks arbitrarios, rutas de cache ni JSON libre.

El Provisioner debe ejecutarse después de cualquier herramienta que pueda
invocar de nuevo el setup de Engram. Para eliminar la disputa de ownership, el
provisioner Codex de `gentle-ai` dejaría de incluir el componente `engram`; se
mantendrían sus otros componentes.

## Alternativas

| Alternativa | Ventaja | Problema | Veredicto |
| --- | --- | --- | --- |
| Parchear cache después de setup | rápida | ownership incorrecto, hashes/layout frágiles | rechazada |
| Versionar hooks e instrucciones completos en dots | control inmediato | dots mantiene una integración runtime ajena | rechazada |
| Ejecutar setup y retirar después lo no deseado | reproduce primero el setup oficial | cada setup/upgrade restaura contenido; no reduce `serverInstructions`; puede dejar integración parcial | útil solo como experimento sandboxed |
| Desactivar plugin y conservar solo MCP | soportada hoy y muy lean | pierde lifecycle, captura de prompts y recuperación automática | experimento inmediato preferible al parche de cache |
| Proxy MCP stdio que filtre `initialize.result.instructions` | conserva herramientas y no toca el cache | añade runtime propio; solo resuelve instrucciones, no hooks ni archivos de setup | experimento viable, no objetivo estable |
| Flag upstream `engram mcp --instructions=...` | cambio pequeño, explícito y reutilizable | requiere aceptación y release upstream | primer cambio upstream recomendado |
| Pasar opciones mediante `gentle-ai` | menor cambio en dots | doble indirección y ownership menos claro | transición posible |
| Perfil upstream + Provisioner `engram` | límite limpio, idempotente y actualizable | requiere coordinación upstream | recomendada |

### Opciones que pueden evaluarse antes del perfil upstream

No se elige todavía una implementación. Se conservan tres experimentos para
una decisión posterior:

1. **MCP-only**: no instalar el plugin Engram para Codex y registrar únicamente
   `engram mcp`. Es la opción soportada más lean, pero renuncia explícitamente
   al lifecycle, la captura automática de prompts y la recuperación automática.
2. **Setup más parche**: ejecutar `engram setup codex` y retirar hooks o
   instrucciones. Sirve para medir comportamiento en un entorno temporal, pero
   no debe convertirse en estado administrado por `dots`.
3. **Proxy stdio**: envolver `engram mcp`, reenviar JSON-RPC y eliminar o
   sustituir solo `result.instructions` en la respuesta `initialize`. Es menos
   dependiente del layout del plugin que parchear el cache, pero convierte el
   proxy en una integración runtime propiedad de `dots`.

Ninguna de estas opciones reproduce por sí sola el perfil `continuity`
completo. En particular, filtrar `serverInstructions` no cambia
`model_instructions_file`, el prompt de compactación ni los hooks que
`engram setup codex` instala.

## Plan de entrega

### Fase 0 — línea base reproducible

- Medir en un `HOME`, `CODEX_HOME` y data dir temporales:
  bytes de contexto emitidos por cada hook, tool calls iniciales, observaciones
  y prompts tras escenarios definidos.
- Escenarios: tarea nueva, reanudación, compactación y tarea con dos subagentes.
- No usar el home ni la base Engram reales.

### Fase 1 — Engram upstream

- Proponer el perfil `continuity`.
- Añadir pruebas de setup idempotente y de cada hook con perfil `full` y
  `continuity`.
- Hacer que MCP y hooks lean lean la misma fuente de política.
- Mantener `full` como default para no romper instalaciones existentes.

### Fase 2 — dots

- Crear issue/ADR para el nuevo Provisioner y el cambio de ownership.
- Añadir validación, renderer, plan/report/metadata y tests del dialecto.
- Probar dos instalaciones consecutivas contra un home temporal.
- Migrar `dots.yaml` solo cuando la versión mínima de Engram soporte el perfil.

### Fase 3 — evaluación

- Ejecutar un delivery real de aproximadamente una hora.
- Comparar continuidad útil, tool calls, observaciones y duplicados contra la
  línea base.
- Promover la política solo si no degrada reanudación o compactación.

## Criterios de aceptación

- `dots install` dos veces deja el mismo estado.
- Ninguna prueba escribe en el home/configuración reales.
- El cache instalado sigue byte-a-byte igual al plugin upstream.
- Una tarea nueva no fuerza `mem_context` ni ToolSearch.
- `SessionStart` normal no añade protocolo estático al modelo.
- Los prompts siguen asociados a la sesión/observación correspondiente.
- Terminar un subagente no crea observaciones pasivas en `continuity`.
- Compactación recupera contexto con una sola instrucción/ruta.
- Una sesión significativa produce un resumen final y solo las observaciones
  semánticas justificadas; no duplica commits, checks o bundles.
- Si upstream cambia el esquema/perfil, el Provisioner falla claramente en vez
  de converger parcialmente.

## Riesgos y preguntas abiertas

- Hay que confirmar si Codex deduplica de forma estable el MCP explícito y el
  MCP empaquetado por el plugin; no se debe inferir solo por ver un namespace.
- Hay que decidir si un experimento MCP-only conserva suficiente valor antes de
  asumir el coste de hooks propios o de coordinación upstream.
- Si se prueba un proxy stdio, debe limitarse a reescribir la respuesta
  estándar `initialize`; no debe interpretar herramientas ni datos de Engram.
- El perfil slim del MCP debe conocerse antes de construir
  `serverInstructions`; de lo contrario las instrucciones agresivas seguirán
  llegando aunque los hooks sean silenciosos.
- Debe definirse qué cuenta como “sesión significativa” sin convertir la
  política en un temporizador o cuota rígida.
- La captura de prompts tiene coste de almacenamiento/privacidad aunque no
  añada tokens; debe poder desactivarse explícitamente.
- Antes de migrar, hay que verificar que `gentle-ai` puede instalar `context7`
  para Codex sin reinstalar Engram.

## Conclusión

Engram sí aporta valor, pero hoy Codex recibe una política repetida y demasiado
activa. La corrección sostenible no es que `dots` reescriba el resultado de
`engram setup`; es hacer que Engram exponga una política Codex lean y que
`dots` la declare mediante un Provisioner cerrado. Eso preserva la división
existente: `dots` es la Source of Truth de la intención y Engram es el dueño
del Regenerated Content.
