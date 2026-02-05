<script lang="ts">
  interface ConversationItem {
    id: string
    title: string
    updatedAt: string
  }

  let {
    conversations,
    activeId,
    onSelect,
    onNew,
    onDelete,
  }: {
    conversations: ConversationItem[]
    activeId: string | null
    onSelect: (id: string) => void
    onNew: () => void
    onDelete: (id: string) => void
  } = $props()

  function relativeDate(iso: string): string {
    const date = new Date(iso)
    const now = new Date()
    const diffMs = now.getTime() - date.getTime()
    const diffMin = Math.floor(diffMs / 60000)
    const diffHr = Math.floor(diffMs / 3600000)
    const diffDay = Math.floor(diffMs / 86400000)

    if (diffMin < 1) return 'just now'
    if (diffMin < 60) return `${diffMin}m ago`
    if (diffHr < 24) return `${diffHr}h ago`
    if (diffDay < 7) return `${diffDay}d ago`
    return date.toLocaleDateString()
  }
</script>

<div class="conversation-list">
  <button class="new-chat-btn" onclick={onNew}>
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <line x1="12" y1="5" x2="12" y2="19"></line>
      <line x1="5" y1="12" x2="19" y2="12"></line>
    </svg>
    New chat
  </button>

  <div class="list">
    {#each conversations as conv (conv.id)}
      <div
        class="conv-item"
        class:active={conv.id === activeId}
        role="button"
        tabindex="0"
        onclick={() => onSelect(conv.id)}
        onkeydown={(e) => e.key === 'Enter' && onSelect(conv.id)}
      >
        <div class="conv-title">{conv.title}</div>
        <div class="conv-meta">
          <span class="conv-date">{relativeDate(conv.updatedAt)}</span>
          <button
            class="delete-btn"
            title="Delete conversation"
            onclick={(e: MouseEvent) => { e.stopPropagation(); onDelete(conv.id); }}
          >
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <line x1="18" y1="6" x2="6" y2="18"></line>
              <line x1="6" y1="6" x2="18" y2="18"></line>
            </svg>
          </button>
        </div>
      </div>
    {/each}

    {#if conversations.length === 0}
      <div class="empty">No conversations yet</div>
    {/if}
  </div>
</div>

<style>
  .conversation-list {
    display: flex;
    flex-direction: column;
    height: 100%;
  }

  .new-chat-btn {
    display: flex;
    align-items: center;
    gap: 8px;
    margin: 12px;
    padding: 8px 12px;
    border: 1px dashed var(--border);
    border-radius: 8px;
    background: none;
    color: var(--text-dim);
    font-size: 13px;
    cursor: pointer;
  }

  .new-chat-btn:hover {
    background: var(--bg-hover);
    color: var(--text);
    border-color: var(--text-dim);
  }

  .list {
    flex: 1;
    overflow-y: auto;
    padding: 0 4px;
  }

  .conv-item {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 8px 12px;
    margin: 0 4px;
    border-radius: 6px;
    cursor: pointer;
    border: none;
    background: none;
    text-align: left;
    width: calc(100% - 8px);
  }

  .conv-item:hover {
    background: var(--bg-hover);
  }

  .conv-item.active {
    background: var(--bg-active);
  }

  .conv-title {
    font-size: 13px;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .conv-meta {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .conv-date {
    font-size: 11px;
    color: var(--text-dim);
  }

  .delete-btn {
    display: none;
    padding: 2px;
    border: none;
    border-radius: 4px;
    background: none;
    color: var(--text-dim);
    cursor: pointer;
  }

  .conv-item:hover .delete-btn {
    display: flex;
  }

  .delete-btn:hover {
    color: var(--error);
    background: var(--bg-hover);
  }

  .empty {
    padding: 16px;
    color: var(--text-dim);
    font-size: 13px;
    text-align: center;
  }
</style>
