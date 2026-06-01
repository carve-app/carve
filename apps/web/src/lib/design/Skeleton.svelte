<script lang="ts">
  export let width: string = '100%';
  export let height: string = '1em';
  export let radius: string = 'var(--r-sm)';
  let className = '';
  export { className as class };
</script>

<span
  class="skel {className}"
  style="width: {width}; height: {height}; border-radius: {radius};"
  aria-hidden="true"
></span>

<style>
  .skel {
    display: inline-block;
    background: linear-gradient(
      90deg,
      var(--c-bgInput) 0%,
      var(--c-border) 50%,
      var(--c-bgInput) 100%
    );
    background-size: 200% 100%;
    animation: shimmer 1.2s linear infinite;
  }
  @keyframes shimmer {
    from { background-position: 200% 0; }
    to   { background-position: -100% 0; }
  }
</style>
