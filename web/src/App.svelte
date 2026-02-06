<script lang="ts">
  import { onMount } from 'svelte'
  import { client } from './lib/graphql/client'
  import {
    LIST_DOCUMENTS,
    GET_ENTITY,
    UPDATE_CONTENT,
    LIST_LABELS,
    UPDATE_ENTITY_LABELS,
  } from './lib/graphql/queries'
  import Sidebar from './lib/components/Sidebar.svelte'
  import Editor from './lib/components/Editor.svelte'
  import SaveStatus from './lib/components/SaveStatus.svelte'
  import ChatPanel from './lib/components/ChatPanel.svelte'
  import LabelBadge from './lib/components/LabelBadge.svelte'
  import LabelCombobox from './lib/components/LabelCombobox.svelte'

  interface EntityListItem {
    id: string
    name: string
    labels: string[]
    updatedAt: string
  }

  interface EntityFull {
    id: string
    name: string
    content: string | null
    labels: string[]
    updatedAt: string
  }

  interface LabelCount {
    label: string
    count: number
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
  let chatOpen = $state(false)
  let allLabels = $state<LabelCount[]>([])
  let filterLabels = $state<string[]>([])
  let labelError = $state<string | null>(null)
  let labelErrorTimeout: ReturnType<typeof setTimeout> | undefined
  let labelOpInFlight = $state(false)

  let isDirty = $derived(editorContent !== lastSavedContent)

  onMount(() => {
    loadLabels()

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

  // Fetch documents on mount and when label filters change
  $effect(() => {
    const vars = filterLabels.length > 0 ? { labels: filterLabels } : {}
    loadDocuments(vars)
  })

  async function loadDocuments(vars: { labels?: string[] } = {}) {
    try {
      loadError = null
      const data: { entities: EntityListItem[] } = await client.request(LIST_DOCUMENTS, vars)
      entities = data.entities
    } catch (e) {
      console.error('Failed to load documents:', e)
      loadError = 'Failed to load documents. Is the server running?'
    }
  }

  async function loadLabels() {
    try {
      const data: { labels: LabelCount[] } = await client.request(LIST_LABELS)
      allLabels = data.labels
    } catch (e) {
      console.error('Failed to load labels:', e)
      showLabelError('Failed to load labels')
    }
  }

  function showLabelError(msg: string) {
    labelError = msg
    if (labelErrorTimeout) clearTimeout(labelErrorTimeout)
    labelErrorTimeout = setTimeout(() => { labelError = null }, 4000)
  }

  async function updateLabel(label: string, mode: 'add' | 'remove') {
    if (!selectedId || !selectedEntity) return
    if (mode === 'add' && selectedEntity.labels.includes(label)) return
    if (labelOpInFlight) return
    labelOpInFlight = true

    const prev = selectedEntity.labels
    selectedEntity = {
      ...selectedEntity,
      labels: mode === 'add' ? [...prev, label] : prev.filter((l) => l !== label),
    }

    try {
      const input = mode === 'add' ? { addLabels: [label] } : { delLabels: [label] }
      const data: { updateEntity: EntityFull } = await client.request(
        UPDATE_ENTITY_LABELS,
        { id: selectedId, input },
      )
      selectedEntity = { ...selectedEntity, labels: data.updateEntity.labels }
      await Promise.all([loadDocuments(), loadLabels()])
    } catch (e) {
      console.error(`Failed to ${mode} label:`, e)
      selectedEntity = { ...selectedEntity, labels: prev }
      showLabelError(`Failed to ${mode} label "${label}"`)
    } finally {
      labelOpInFlight = false
    }
  }

  function addLabel(label: string) { updateLabel(label, 'add') }
  function removeLabel(label: string) { updateLabel(label, 'remove') }

  function toggleFilterLabel(label: string) {
    if (filterLabels.includes(label)) {
      filterLabels = filterLabels.filter((l) => l !== label)
    } else {
      filterLabels = [...filterLabels, label]
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
  <Sidebar
    {entities}
    {selectedId}
    {allLabels}
    {filterLabels}
    onSelect={selectEntity}
    onToggleFilter={toggleFilterLabel}
  />

  <main class="editor-pane">
    {#if selectedEntity}
      <div class="toolbar">
        <div class="toolbar-top">
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
            <button
              class="chat-toggle"
              onclick={() => chatOpen = !chatOpen}
              title="Toggle chat"
              class:active={chatOpen}
            >
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"></path>
              </svg>
            </button>
          </div>
        </div>
        <div class="toolbar-labels">
          {#each selectedEntity.labels as label (label)}
            <LabelBadge {label} removable onremove={() => removeLabel(label)} />
          {/each}
          <LabelCombobox
            {allLabels}
            currentLabels={selectedEntity.labels}
            onAdd={addLabel}
          />
          {#if labelError}
            <span class="label-error">{labelError}</span>
          {/if}
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
          <div class="empty-content">
            <span>Select a document to edit</span>
            <button
              class="chat-toggle standalone"
              onclick={() => chatOpen = !chatOpen}
              title="Open chat"
            >
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"></path>
              </svg>
              Chat
            </button>
          </div>
        {/if}
      </div>
    {/if}
  </main>
</div>

<ChatPanel
  open={chatOpen}
  onClose={() => chatOpen = false}
  entityId={selectedId}
  entityLabels={selectedEntity?.labels ?? []}
/>

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
    flex-direction: column;
    padding: 8px 16px;
    border-bottom: 1px solid var(--border);
    background: var(--bg-surface);
    gap: 6px;
  }

  .toolbar-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
  }

  .toolbar-labels {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 4px;
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

  .chat-toggle {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 6px;
    width: 32px;
    height: 32px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--bg);
    color: var(--text-dim);
    cursor: pointer;
  }

  .chat-toggle:hover {
    background: var(--bg-hover);
    color: var(--text);
  }

  .chat-toggle.active {
    background: var(--bg-active);
    color: var(--accent);
    border-color: var(--accent);
  }

  .chat-toggle.standalone {
    width: auto;
    padding: 6px 12px;
    font-size: 13px;
    margin-top: 12px;
  }

  .empty-state {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--text-dim);
    font-size: 15px;
  }

  .empty-content {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 4px;
  }

  .error-text {
    color: var(--error);
  }

  .label-error {
    font-size: 11px;
    color: var(--error);
  }
</style>
