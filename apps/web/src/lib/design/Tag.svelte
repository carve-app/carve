<script lang="ts">
  export let variant: 'default' | 'success' | 'warning' | 'danger' | 'info' = 'default';
  export let size: 'sm' | 'md' = 'sm';
  let className = '';
  export { className as class };
</script>

<span class="tag {variant} {size} {className}"><slot /></span>

<style>
  .tag {
    display: inline-flex;
    align-items: center;
    border-radius: var(--r-sm);
    font-weight: 600;
    line-height: 1;
    white-space: nowrap;
  }
  .sm { padding: 0.15rem 0.45rem; font-size: 0.72rem; }
  .md { padding: 0.25rem 0.6rem;  font-size: 0.78rem; }

  .default { background: var(--c-border);  color: var(--c-text); }
  .success { background: color-mix(in srgb, var(--c-green)  20%, transparent); color: var(--c-greenLite); }
  .warning { background: color-mix(in srgb, var(--c-warning) 20%, transparent); color: #ffcc80; }
  .danger  { background: color-mix(in srgb, var(--c-danger)  20%, transparent); color: #ef9a9a; }
  .info    { background: color-mix(in srgb, var(--c-info)    20%, transparent); color: #90caf9; }
</style>
