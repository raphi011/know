<script lang="ts">
  import { marked } from 'marked'

  interface ChatMessage {
    id: string
    role: string
    content: string
  }

  let {
    messages,
    streamingContent = '',
  }: {
    messages: ChatMessage[]
    streamingContent?: string
  } = $props()

  let container: HTMLDivElement

  // Auto-scroll to bottom when messages change or streaming updates
  $effect(() => {
    // Track dependencies
    messages.length
    streamingContent
    // Scroll after DOM updates
    if (container) {
      requestAnimationFrame(() => {
        container.scrollTop = container.scrollHeight
      })
    }
  })

  function renderMarkdown(text: string): string {
    return marked.parse(text, { async: false }) as string
  }
</script>

<div class="message-list" bind:this={container}>
  {#each messages as msg (msg.id)}
    <div class="message" class:user={msg.role === 'user'} class:assistant={msg.role === 'assistant'}>
      {#if msg.role === 'user'}
        <div class="bubble user-bubble">{msg.content}</div>
      {:else}
        <div class="bubble assistant-bubble">{@html renderMarkdown(msg.content)}</div>
      {/if}
    </div>
  {/each}

  {#if streamingContent}
    <div class="message assistant">
      <div class="bubble assistant-bubble">
        {@html renderMarkdown(streamingContent)}
        <span class="cursor"></span>
      </div>
    </div>
  {/if}

  {#if messages.length === 0 && !streamingContent}
    <div class="empty">Ask a question about your documents</div>
  {/if}
</div>

<style>
  .message-list {
    flex: 1;
    overflow-y: auto;
    padding: 16px 12px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .message {
    display: flex;
  }

  .message.user {
    justify-content: flex-end;
  }

  .message.assistant {
    justify-content: flex-start;
  }

  .bubble {
    max-width: 85%;
    padding: 8px 12px;
    border-radius: 12px;
    font-size: 13px;
    line-height: 1.5;
    word-break: break-word;
  }

  .user-bubble {
    background: var(--accent);
    color: var(--bg);
    border-bottom-right-radius: 4px;
    white-space: pre-wrap;
  }

  .assistant-bubble {
    background: var(--bg-hover);
    color: var(--text);
    border-bottom-left-radius: 4px;
  }

  /* Markdown content styling */
  .assistant-bubble :global(p) {
    margin: 0 0 8px 0;
  }

  .assistant-bubble :global(p:last-child) {
    margin-bottom: 0;
  }

  .assistant-bubble :global(pre) {
    background: var(--bg);
    padding: 8px;
    border-radius: 6px;
    overflow-x: auto;
    margin: 8px 0;
    font-size: 12px;
  }

  .assistant-bubble :global(code) {
    font-family: 'SF Mono', 'Fira Code', monospace;
    font-size: 12px;
  }

  .assistant-bubble :global(ul),
  .assistant-bubble :global(ol) {
    padding-left: 20px;
    margin: 4px 0;
  }

  .assistant-bubble :global(li) {
    margin: 2px 0;
  }

  .cursor {
    display: inline-block;
    width: 2px;
    height: 14px;
    background: var(--accent);
    margin-left: 2px;
    vertical-align: text-bottom;
    animation: blink 1s step-end infinite;
  }

  @keyframes blink {
    50% { opacity: 0; }
  }

  .empty {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--text-dim);
    font-size: 14px;
  }
</style>
