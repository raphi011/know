<script lang="ts">
  import LabelBadge from './LabelBadge.svelte'

  interface Entity {
    id: string
    name: string
    labels: string[]
    updatedAt: string
  }

  let {
    entities,
    selectedId,
    allLabels,
    filterLabels,
    onSelect,
    onToggleFilter,
  }: {
    entities: Entity[]
    selectedId: string | null
    allLabels: { label: string; count: number }[]
    filterLabels: string[]
    onSelect: (id: string) => void
    onToggleFilter: (label: string) => void
  } = $props()

  let search = $state('')

  let filtered = $derived(
    search
      ? entities.filter((e) =>
          e.name.toLowerCase().includes(search.toLowerCase()),
        )
      : entities,
  )
</script>

<aside class="sidebar">
  {#if allLabels.length > 0}
    <div class="filter-chips">
      {#each allLabels as { label } (label)}
        <LabelBadge
          {label}
          selected={filterLabels.includes(label)}
          onclick={() => onToggleFilter(label)}
        />
      {/each}
    </div>
  {/if}
  <div class="search-box">
    <input
      type="text"
      placeholder="Filter documents..."
      bind:value={search}
    />
  </div>
  <div class="entity-list">
    {#each filtered as entity (entity.id)}
      <button
        class="entity-item"
        class:active={entity.id === selectedId}
        onclick={() => onSelect(entity.id)}
      >
        <span class="entity-name">{entity.name}</span>
        {#if entity.labels.length > 0}
          <span class="entity-labels">
            {#each entity.labels as label (label)}
              <LabelBadge {label} />
            {/each}
          </span>
        {/if}
      </button>
    {/each}
    {#if filtered.length === 0}
      <div class="empty">
        {search ? 'No matches' : 'No documents'}
      </div>
    {/if}
  </div>
</aside>

<style>
  .sidebar {
    width: var(--sidebar-width);
    min-width: var(--sidebar-width);
    height: 100%;
    background: var(--bg-surface);
    border-right: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  .filter-chips {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
    padding: 10px 12px;
    border-bottom: 1px solid var(--border);
  }

  .search-box {
    padding: 12px;
    border-bottom: 1px solid var(--border);
  }

  .search-box input {
    width: 100%;
    padding: 8px 12px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--bg);
    color: var(--text);
    font-size: 13px;
    outline: none;
  }

  .search-box input:focus {
    border-color: var(--accent);
  }

  .entity-list {
    flex: 1;
    overflow-y: auto;
    padding: 4px 0;
  }

  .entity-item {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 4px;
    width: 100%;
    padding: 8px 16px;
    border: none;
    background: none;
    color: var(--text);
    font-size: 13px;
    text-align: left;
    cursor: pointer;
  }

  .entity-name {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 100%;
  }

  .entity-labels {
    display: flex;
    flex-wrap: wrap;
    gap: 3px;
  }

  .entity-item:hover {
    background: var(--bg-hover);
  }

  .entity-item.active {
    background: var(--bg-active);
    color: var(--accent);
  }

  .empty {
    padding: 16px;
    color: var(--text-dim);
    font-size: 13px;
    text-align: center;
  }
</style>
