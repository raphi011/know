<script lang="ts">
  let {
    onSend,
    disabled = false,
  }: {
    onSend: (text: string) => void
    disabled?: boolean
  } = $props()

  let text = $state('')
  let textarea: HTMLTextAreaElement

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      send()
    }
  }

  function send() {
    const trimmed = text.trim()
    if (!trimmed || disabled) return
    onSend(trimmed)
    text = ''
    // Reset textarea height
    if (textarea) textarea.style.height = 'auto'
  }

  function autoResize() {
    if (!textarea) return
    textarea.style.height = 'auto'
    textarea.style.height = Math.min(textarea.scrollHeight, 120) + 'px'
  }
</script>

<div class="chat-input">
  <textarea
    bind:this={textarea}
    bind:value={text}
    onkeydown={handleKeydown}
    oninput={autoResize}
    placeholder="Ask a question..."
    rows="1"
    {disabled}
  ></textarea>
  <button
    class="send-btn"
    onclick={send}
    disabled={disabled || !text.trim()}
    title="Send message"
  >
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <line x1="22" y1="2" x2="11" y2="13"></line>
      <polygon points="22 2 15 22 11 13 2 9 22 2"></polygon>
    </svg>
  </button>
</div>

<style>
  .chat-input {
    display: flex;
    align-items: flex-end;
    gap: 8px;
    padding: 12px;
    border-top: 1px solid var(--border);
    background: var(--bg-surface);
  }

  textarea {
    flex: 1;
    padding: 8px 12px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--bg);
    color: var(--text);
    font-family: inherit;
    font-size: 13px;
    line-height: 1.4;
    resize: none;
    outline: none;
    min-height: 36px;
    max-height: 120px;
  }

  textarea:focus {
    border-color: var(--accent);
  }

  textarea:disabled {
    opacity: 0.5;
  }

  .send-btn {
    width: 36px;
    height: 36px;
    border: none;
    border-radius: 8px;
    background: var(--accent);
    color: var(--bg);
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }

  .send-btn:hover:not(:disabled) {
    opacity: 0.9;
  }

  .send-btn:disabled {
    opacity: 0.3;
    cursor: default;
  }
</style>
