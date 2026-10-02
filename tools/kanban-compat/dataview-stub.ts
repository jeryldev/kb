// Dataview is not installed in the vault kb targets; the plugin then skips
// inline-field parsing, which is the case this harness checks.
export function getAPI() {
  return undefined;
}
