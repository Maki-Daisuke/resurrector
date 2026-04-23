import sveltePreprocess from 'svelte-preprocess'

export default {
  // Consult https://github.com/sveltejs/svelte-preprocess
  // for more information about preprocessors
  preprocess: sveltePreprocess(),

  // Enable runes mode across the whole project so any legacy Svelte 3/4
  // syntax (export let, $:, on:click, <slot>, ...) fails to compile. This
  // mirrors the setting in vite.config.ts and is what svelte-check reads.
  compilerOptions: { runes: true },
}
