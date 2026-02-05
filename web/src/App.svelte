<script lang="ts">
  import { onMount } from 'svelte'
  import { client } from './lib/graphql/client'
  import { LIST_DOCUMENTS, GET_ENTITY, UPDATE_CONTENT } from './lib/graphql/queries'
  import Sidebar from './lib/components/Sidebar.svelte'
  import Editor from './lib/components/Editor.svelte'
  import SaveStatus from './lib/components/SaveStatus.svelte'

  interface EntityListItem {
    id: string
    name: string
    updatedAt: string
  }

  interface EntityFull {
    id: string
    name: string
    content: string | null
    updatedAt: string
  }

  let entities = $state<EntityListItem[]>([])
  let selectedId = $state<string | null>(null)
  let selectedEntity = $state<EntityFull | null>(null)
  let editorContent = $state('')
  let lastSavedContent = $state('')
  let saveStatus = $state<'idle' | 'saving' | 'saved' | 'error'>('idle')
  let saveTimeout: ReturnType<typeof setTimeout> | undefined
  let loading = $state(false)
  let loadError = $state<string | null>(null)

  let isDirty = $derived(editorContent !== lastSavedContent)

  onMount(() => {
    loadDocuments()

    // Global Cmd/Ctrl+S handler for when editor doesn't have focus
    function handleKeydown(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key === 's') {
        e.preventDefault()
        save()
      }
    }
    document.addEventListener('keydown', handleKeydown)
    return () => document.removeEventListener('keydown', handleKeydown)
  })

  async function loadDocuments() {
    try {
      loadError = null
      const data: { entities: EntityListItem[] } = await client.request(LIST_DOCUMENTS)
      entities = data.entities
    } catch (e) {
      console.error('Failed to load documents:', e)
      loadError = 'Failed to load documents. Is the server running?'
    }
  }

  async function selectEntity(id: string) {
    if (id === selectedId) return

    // Warn if unsaved changes would be lost
    if (isDirty && !confirm('You have unsaved changes. Discard them?')) {
      return
    }

    selectedId = id
    loading = true
    try {
      const data: { entity: EntityFull } = await client.request(GET_ENTITY, { id })
      if (!data.entity) {
        selectedEntity = null
        editorContent = ''
        lastSavedContent = ''
        return
      }
      selectedEntity = data.entity
      const content = data.entity.content ?? ''
      editorContent = content
      lastSavedContent = content
      saveStatus = 'idle'
    } catch (e) {
      console.error('Failed to load entity:', e)
      selectedId = null
      selectedEntity = null
    } finally {
      loading = false
    }
  }

  async function save() {
    if (!selectedId || !isDirty) return

    // Snapshot before async boundary to avoid race with continued edits
    const contentToSave = editorContent
    const entityId = selectedId

    saveStatus = 'saving'
    if (saveTimeout) clearTimeout(saveTimeout)

    try {
      const data: { updateEntityContent: EntityFull } = await client.request(
        UPDATE_CONTENT,
        { id: entityId, content: contentToSave },
      )
      lastSavedContent = contentToSave
      selectedEntity = data.updateEntityContent
      saveStatus = 'saved'
      saveTimeout = setTimeout(() => {
        saveStatus = 'idle'
      }, 3000)
    } catch (e) {
      console.error('Save failed:', e)
      saveStatus = 'error'
    }
  }
</script>

<div class="layout">
  <Sidebar {entities} {selectedId} onSelect={selectEntity} />

  <main class="editor-pane">
    {#if selectedEntity}
      <div class="toolbar">
        <span class="doc-name">{selectedEntity.name}</span>
        <div class="toolbar-right">
          <SaveStatus status={saveStatus} />
          <button
            class="save-btn"
            onclick={save}
            disabled={!isDirty || saveStatus === 'saving'}
          >
            Save
          </button>
        </div>
      </div>
      <Editor
        content={editorContent}
        onChange={(c: string) => editorContent = c}
        onSave={save}
      />
    {:else}
      <div class="empty-state">
        {#if loading}
          Loading...
        {:else if loadError}
          <span class="error-text">{loadError}</span>
        {:else}
          Select a document to edit
        {/if}
      </div>
    {/if}
  </main>
</div>

<style>
  .layout {
    display: flex;
    height: 100%;
  }

  .editor-pane {
    flex: 1;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    min-width: 0;
  }

  .toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 8px 16px;
    border-bottom: 1px solid var(--border);
    background: var(--bg-surface);
    gap: 12px;
  }

  .doc-name {
    font-size: 14px;
    font-weight: 500;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
  }

  .toolbar-right {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-shrink: 0;
  }

  .save-btn {
    padding: 4px 16px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--bg);
    color: var(--text);
    font-size: 13px;
    cursor: pointer;
  }

  .save-btn:hover:not(:disabled) {
    background: var(--bg-hover);
  }

  .save-btn:disabled {
    opacity: 0.4;
    cursor: default;
  }

  .empty-state {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--text-dim);
    font-size: 15px;
  }

  .error-text {
    color: var(--error);
  }
</style>
