<script lang="ts">
  /**
   * Calendar heatmap (GitHub-contributions style). Renders the last
   * `weeks` weeks as a grid of 7×N cells; each cell's opacity tracks
   * its value vs. the max in the data set.
   *
   * Pass `data` as { date: 'YYYY-MM-DD', value: number }[]. Missing
   * dates render as the no-activity color.
   */
  export let data: { date: string; value: number }[] = [];
  export let weeks = 13;
  export let cellSize = 12;
  export let gap = 3;
  export let title: string | null = null;

  $: today = new Date();
  $: startDay = (() => {
    const d = new Date(today);
    d.setDate(d.getDate() - weeks * 7 + 1);
    return d;
  })();

  function fmt(d: Date) { return d.toISOString().slice(0, 10); }

  $: dataMap = new Map(data.map((d) => [d.date, d.value]));
  $: maxVal = Math.max(1, ...data.map((d) => d.value));

  function cells() {
    const out: { date: string; value: number; opacity: number }[] = [];
    const cur = new Date(startDay);
    while (cur <= today) {
      const date = fmt(cur);
      const value = dataMap.get(date) ?? 0;
      const opacity = value === 0 ? 0.08 : 0.25 + 0.75 * (value / maxVal);
      out.push({ date, value, opacity });
      cur.setDate(cur.getDate() + 1);
    }
    return out;
  }

  $: gridCells = cells();
  $: width  = weeks * (cellSize + gap);
  $: height = 7 * (cellSize + gap);
</script>

<figure>
  {#if title}<figcaption>{title}</figcaption>{/if}
  <svg {width} {height} role="img" aria-label={title ?? 'activity heatmap'}>
    {#each gridCells as cell, i}
      {@const week = Math.floor(i / 7)}
      {@const day  = i % 7}
      <rect
        x={week * (cellSize + gap)}
        y={day  * (cellSize + gap)}
        width={cellSize}
        height={cellSize}
        rx="2"
        fill="var(--c-green)"
        fill-opacity={cell.opacity}
      >
        <title>{cell.date}: {cell.value}</title>
      </rect>
    {/each}
  </svg>
</figure>

<style>
  figure { margin: 0; }
  figcaption { font-size: 0.78rem; color: var(--c-textMuted); margin-bottom: var(--s-2); }
  svg { display: block; }
</style>
