// Just enough of Obsidian's API for the Kanban plugin's parser to run in
// Node: YAML (Obsidian uses the same yaml library) and moment are real; the
// rest are empty stand-ins for names the bundle imports but the parse and
// save path never calls.
import moment from 'moment';
import YAML from 'yaml';

export function parseYaml(text: string) {
  return YAML.parse(text) ?? {};
}
export function stringifyYaml(value: unknown) {
  return YAML.stringify(value);
}
export { moment };
export class App {}
export class Component {}
export class DropdownComponent {}
export class Editor {}
export class EditorPosition {}
export class EditorSuggest {}
export class EditorSuggestContext {}
export class EditorSuggestTriggerInfo {}
export class HoverParent {}
export class HoverPopover {}
export class Keymap {}
export class MarkdownRenderer {}
export class MarkdownView {}
export class Menu {}
export class Modal {}
export class Platform {}
export class Plugin {}
export class PluginSettingTab {}
export class Setting {}
export class Stat {}
export class TFile {}
export class TFolder {}
export class TextFileView {}
export class ToggleComponent {}
export class Vault {}
export class ViewState {}
export class ViewStateResult {}
export class WorkspaceLeaf {}
export function debounce(fn: any) {
  return fn;
}
export function getLinkpath(link: string) {
  return link;
}
export function htmlToMarkdown(s: string) {
  return s;
}
export function parseLinktext(link: string) {
  return { path: link, subpath: '' };
}
export function setIcon() {}
