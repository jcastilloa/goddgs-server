## ADDED Requirements

### Requirement: Registro de selección de fuentes de research
El sistema SHALL registrar un paso `research_selection` correlacionado con cada operación de research que alcance la selección previa a extracción. El paso MUST guardar su estado y duración junto con el número de candidatos descubiertos y seleccionados, y MUST NOT persistir URLs, títulos, descripciones, prompts ni respuestas completas del LLM.

#### Scenario: Selección previa completada
- **WHEN** una operación de research entrega candidatos de búsqueda al selector y recibe una selección válida
- **THEN** el almacenamiento conserva un paso `research_selection` completado con su duración y ambos conteos, asociado a la operación raíz

#### Scenario: Fallo del selector
- **WHEN** la llamada al LLM selector falla o devuelve una selección inválida
- **THEN** el paso `research_selection` se marca como fallido con un error saneado y no persiste metadatos de candidatos ni contenido del LLM
