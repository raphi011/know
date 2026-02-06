<script lang="ts">
  let {
    allLabels,
    currentLabels,
    onAdd,
  }: {
    allLabels: { label: string; count: number }[]
    currentLabels: string[]
    onAdd: (label: string) => void
  } = $props()

  let query = $state('')
  let open = $state(false)
  let highlightIndex = $state(0)
  let inputEl: HTMLInputElement | undefined = $state()

  let suggestions = $derived.by(() => {
    const q = query.toLowerCase().trim()
    const available = allLabels
      .filter((l) => !currentLabels.includes(l.label))
      .filter((l) => !q || l.label.toLowerCase().includes(q))
      .map((l) => l.label)

    const items: { label: string; isNew: boolean }[] = available.map((l) => ({
      label: l,
      isNew: false,
    }))

    if (q && !allLabels.some((l) => l.label.toLowerCase() === q)) {
      items.push({ label: q, isNew: true })
    }

    return items
  })

  function select(label: string) {
    onAdd(label)
    query = ''
    highlightIndex = 0
    inputEl?.focus()
  }

  function handleKeydown(e: KeyboardEvent) {
    if (!open && e.key !== 'Escape') {
      open = true
    }

    if (e.key === 'ArrowDown') {
      e.preventDefault()
      highlightIndex = Math.min(highlightIndex + 1, suggestions.length - 1)
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      highlightIndex = Math.max(highlightIndex - 1, 0)
    } else if (e.key === 'Enter') {
      e.preventDefault()
      if (suggestions.length > 0) {
        select(suggestions[highlightIndex].label)
      }
      open = false
    } else if (e.key === 'Escape') {
      open = false
    }
  }

  function handleInput() {
    open = true
    highlightIndex = 0
  }

  function handleFocus() {
    open = true
  }

  function handleBlur() {
    // Delay to allow click on suggestion to register
    setTimeout(() => {
      open = false
    }, 150)
  }
</script>

<div class="combobox">
  <input
    bind:this={inputEl}
    type="text"
    placeholder="Add label..."
    bind:value={query}
    onkeydown={handleKeydown}
    oninput={handleInput}
    onfocus={handleFocus}
    onblur={handleBlur}
  />
  {#if open && suggestions.length > 0}
    <ul class="dropdown">
      {#each suggestions as item, i (item.label + item.isNew)}
        <li>
          <button
            class="dropdown-item"
            class:highlighted={i === highlightIndex}
            onmousedown={() => select(item.label)}
            onmouseenter={() => highlightIndex = i}
          >
            {#if item.isNew}
              Create "{item.label}"
            {:else}
              {item.label}
            {/if}
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .combobox {
    position: relative;
  }

  .combobox input {
    width: 120px;
    padding: 3px 8px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--bg);
    color: var(--text);
    font-size: 12px;
    outline: none;
    font-family: inherit;
  }

  .combobox input:focus {
    border-color: var(--accent);
  }

  .dropdown {
    position: absolute;
    top: 100%;
    left: 0;
    margin-top: 4px;
    min-width: 160px;
    max-height: 200px;
    overflow-y: auto;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 6px;
    list-style: none;
    z-index: 100;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.2);
  }

  .dropdown-item {
    display: block;
    width: 100%;
    padding: 6px 10px;
    border: none;
    background: none;
    color: var(--text);
    font-size: 12px;
    text-align: left;
    cursor: pointer;
    font-family: inherit;
  }

  .dropdown-item:hover,
  .dropdown-item.highlighted {
    background: var(--bg-hover);
  }
</style>
