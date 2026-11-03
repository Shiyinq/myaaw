/**
 * The type of a lucide icon, used wherever a component takes an icon as a prop.
 *
 * Lucide still ships class components, so this is the constructor type of its
 * shared `Icon` class rather than Svelte 5's `Component<Props>`, which only
 * describes function components. Every lucide icon takes the same props, so
 * one alias covers all of them.
 */
export type LucideIcon = typeof import('lucide-svelte').Icon;
