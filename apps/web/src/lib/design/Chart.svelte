<script lang="ts">
  /**
   * SVG line/bar chart primitive. Designed to look clean at 220×60 (a
   * sparkline) up through 720×360 (a full dashboard tile). Renders both
   * axes when `axes={true}` and a tooltip + hover ring on data points.
   */
  export let data: { label: string; value: number }[] = [];
  export let kind: 'line' | 'bar' = 'line';
  export let width = 360;
  export let height = 180;
  export let strokeColor = 'var(--c-green)';
  export let fillColor   = 'color-mix(in srgb, var(--c-green) 18%, transparent)';
  export let axes = true;
  export let yLabel: string | null = null;

  $: padL = axes ? 36 : 8;
  $: padR = 8;
  $: padT = 12;
  $: padB = axes ? 22 : 8;
  $: innerW = Math.max(1, width  - padL - padR);
  $: innerH = Math.max(1, height - padT - padB);

  $: maxVal = data.length ? Math.max(1, ...data.map((d) => d.value)) : 1;
  $: minVal = 0;

  function x(i: number) { return padL + (data.length <= 1 ? innerW / 2 : (i / (data.length - 1)) * innerW); }
  function y(v: number) { return padT + innerH - ((v - minVal) / (maxVal - minVal)) * innerH; }

  $: linePath = data
    .map((d, i) => `${i === 0 ? 'M' : 'L'} ${x(i).toFixed(1)} ${y(d.value).toFixed(1)}`)
    .join(' ');
  $: areaPath = data.length
    ? `${linePath} L ${x(data.length - 1).toFixed(1)} ${(padT + innerH).toFixed(1)} L ${x(0).toFixed(1)} ${(padT + innerH).toFixed(1)} Z`
    : '';

  let hoverIdx: number | null = null;

  function onMove(e: MouseEvent) {
    if (!data.length) return;
    const target = e.currentTarget as SVGSVGElement;
    const rect = target.getBoundingClientRect();
    const relX = e.clientX - rect.left - padL;
    const i = Math.round((relX / innerW) * (data.length - 1));
    hoverIdx = Math.max(0, Math.min(data.length - 1, i));
  }
  function onLeave() { hoverIdx = null; }
</script>

<svg
  width={width}
  height={height}
  viewBox={`0 0 ${width} ${height}`}
  role="img"
  aria-label={yLabel ? `${yLabel} over time` : 'chart'}
  on:mousemove={onMove}
  on:mouseleave={onLeave}
>
  {#if axes}
    <!-- y-axis tick line -->
    <line x1={padL} y1={padT} x2={padL} y2={padT + innerH} stroke="var(--c-border)" />
    <!-- x-axis line -->
    <line x1={padL} y1={padT + innerH} x2={padL + innerW} y2={padT + innerH} stroke="var(--c-border)" />
    <!-- y labels -->
    <text x={padL - 6} y={padT + 4} text-anchor="end" fill="var(--c-textMuted)" font-size="10">{maxVal}</text>
    <text x={padL - 6} y={padT + innerH} text-anchor="end" fill="var(--c-textMuted)" font-size="10">0</text>
    <!-- x labels (first/last only to keep things uncluttered) -->
    {#if data.length}
      <text x={x(0)} y={padT + innerH + 14} fill="var(--c-textMuted)" font-size="10">{data[0].label}</text>
      <text x={x(data.length - 1)} y={padT + innerH + 14} fill="var(--c-textMuted)" font-size="10" text-anchor="end">
        {data[data.length - 1].label}
      </text>
    {/if}
  {/if}

  {#if kind === 'line' && data.length}
    <path d={areaPath} fill={fillColor} />
    <path d={linePath} fill="none" stroke={strokeColor} stroke-width="2" />
    {#each data as d, i}
      <circle cx={x(i)} cy={y(d.value)} r={hoverIdx === i ? 4 : 2.5}
              fill={strokeColor} stroke="var(--c-bgRaised)" stroke-width="1.5" />
    {/each}
  {:else if kind === 'bar'}
    {#each data as d, i}
      <rect
        x={x(i) - 6}
        y={y(d.value)}
        width="12"
        height={Math.max(0, padT + innerH - y(d.value))}
        rx="2"
        fill={strokeColor}
        opacity={hoverIdx === i ? 1 : 0.85}
      />
    {/each}
  {/if}

  {#if hoverIdx != null && data[hoverIdx]}
    <g transform={`translate(${x(hoverIdx)},${y(data[hoverIdx].value) - 12})`}>
      <rect x="-28" y="-18" width="56" height="18" rx="3" fill="var(--c-bgRaised)" stroke="var(--c-border)" />
      <text x="0" y="-5" text-anchor="middle" font-size="11" fill="var(--c-textHi)">
        {data[hoverIdx].label}: {data[hoverIdx].value}
      </text>
    </g>
  {/if}
</svg>

<style>
  svg { display: block; }
</style>
