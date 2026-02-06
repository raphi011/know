<script lang="ts">
  let {
    label,
    removable = false,
    selected = false,
    onclick,
    onremove,
  }: {
    label: string
    removable?: boolean
    selected?: boolean
    onclick?: () => void
    onremove?: () => void
  } = $props()
</script>

{#if onclick}
  <button
    class="badge clickable"
    class:selected
    onclick={onclick}
  >
    {label}
  </button>
{:else}
  <span class="badge">
    {label}
    {#if removable && onremove}
      <button
        class="remove-btn"
        onclick={(e: MouseEvent) => { e.stopPropagation(); onremove!() }}
        aria-label="Remove label {label}"
      >&times;</button>
    {/if}
  </span>
{/if}

<style>
  .badge {
    display: inline-flex;
    align-items: center;
    gap: 2px;
    padding: 2px 8px;
    border-radius: 10px;
    font-size: 11px;
    line-height: 1.4;
    border: 1px solid var(--border);
    background: var(--bg);
    color: var(--text-dim);
    white-space: nowrap;
    font-family: inherit;
  }

  .badge.clickable {
    cursor: pointer;
  }

  .badge.clickable:hover {
    background: var(--bg-hover);
    color: var(--text);
  }

  .badge.selected {
    background: var(--bg-active);
    color: var(--text);
    border-color: var(--text-dim);
  }

  .remove-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 14px;
    height: 14px;
    border: none;
    background: none;
    color: var(--text-dim);
    cursor: pointer;
    padding: 0;
    font-size: 13px;
    line-height: 1;
    border-radius: 50%;
  }

  .remove-btn:hover {
    color: var(--error);
    background: var(--bg-hover);
  }
</style>
