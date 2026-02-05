<script lang="ts">
  import { client } from '../graphql/client'
  import { wsClient } from '../graphql/subscription'
  import {
    LIST_CONVERSATIONS,
    GET_CONVERSATION,
    CREATE_CONVERSATION,
    DELETE_CONVERSATION,
    CHAT_STREAM,
  } from '../graphql/queries'
  import ConversationList from './ConversationList.svelte'
  import MessageList from './MessageList.svelte'
  import ChatInput from './ChatInput.svelte'

  interface ConversationItem {
    id: string
    title: string
    entityId: string | null
    updatedAt: string
  }

  interface ChatMessage {
    id: string
    role: string
    content: string
  }

  let {
    open,
    onClose,
    entityId = null,
    entityLabels = [],
  }: {
    open: boolean
    onClose: () => void
    entityId?: string | null
    entityLabels?: string[]
  } = $props()

  let conversations = $state<ConversationItem[]>([])
  let activeConversationId = $state<string | null>(null)
  let messages = $state<ChatMessage[]>([])
  let isStreaming = $state(false)
  let streamingContent = $state('')
  let msgCounter = $state(0) // for generating temp IDs
  let inChatView = $state(false)

  $effect(() => {
    if (open) {
      loadConversations()
    }
  })

  async function loadConversations() {
    try {
      const data: { conversations: ConversationItem[] } = await client.request(
        LIST_CONVERSATIONS,
        { limit: 50 },
      )
      conversations = data.conversations
    } catch (e) {
      console.error('Failed to load conversations:', e)
    }
  }

  async function selectConversation(id: string) {
    activeConversationId = id
    inChatView = true
    try {
      const data: {
        conversation: {
          id: string
          title: string
          entityId: string | null
          messages: ChatMessage[]
        } | null
      } = await client.request(GET_CONVERSATION, { id })
      if (data.conversation) {
        messages = data.conversation.messages
      }
    } catch (e) {
      console.error('Failed to load conversation:', e)
    }
  }

  function startNewChat() {
    activeConversationId = null
    messages = []
    streamingContent = ''
    inChatView = true
  }

  async function deleteConversation(id: string) {
    try {
      await client.request(DELETE_CONVERSATION, { id })
      conversations = conversations.filter((c) => c.id !== id)
      if (activeConversationId === id) {
        startNewChat()
      }
    } catch (e) {
      console.error('Failed to delete conversation:', e)
    }
  }

  function backToList() {
    activeConversationId = null
    messages = []
    streamingContent = ''
    inChatView = false
    loadConversations()
  }

  async function sendMessage(text: string) {
    if (isStreaming) return

    // Optimistically add user message
    const userMsg: ChatMessage = {
      id: `temp-${++msgCounter}`,
      role: 'user',
      content: text,
    }
    messages = [...messages, userMsg]

    // Create conversation if needed
    let convId = activeConversationId
    if (!convId) {
      try {
        const title = text.length > 80 ? text.slice(0, 77) + '...' : text
        const data: { createConversation: ConversationItem } =
          await client.request(CREATE_CONVERSATION, {
            title,
            entityId: entityId,
          })
        convId = data.createConversation.id
        activeConversationId = convId
      } catch (e) {
        console.error('Failed to create conversation:', e)
        return
      }
    }

    // Build history from existing messages (excluding the one we just added)
    const history = messages.slice(0, -1).map((m) => ({
      role: m.role,
      content: m.content,
    }))

    // Build search input with labels from current document
    const input: { query: string; labels?: string[] } = { query: text }
    if (entityLabels.length > 0) {
      input.labels = entityLabels
    }

    // Start streaming
    isStreaming = true
    streamingContent = ''

    const unsubscribe = wsClient.subscribe(
      {
        query: CHAT_STREAM,
        variables: {
          conversationId: convId,
          message: text,
          history,
          input,
        },
      },
      {
        next(value) {
          const event = value.data?.chatStream
          if (!event) return

          if (event.token) {
            streamingContent += event.token
          }

          if (event.done) {
            // Streaming complete — add assistant message
            if (streamingContent) {
              const assistantMsg: ChatMessage = {
                id: `temp-${++msgCounter}`,
                role: 'assistant',
                content: streamingContent,
              }
              messages = [...messages, assistantMsg]
            }
            streamingContent = ''
            isStreaming = false
            unsubscribe()
          }

          if (event.error) {
            console.error('Chat stream error:', event.error)
            streamingContent = ''
            isStreaming = false
            unsubscribe()
          }
        },
        error(err) {
          console.error('Chat stream WS error:', err)
          streamingContent = ''
          isStreaming = false
        },
        complete() {
          isStreaming = false
        },
      },
    )
  }
</script>

<div class="chat-panel" class:open>
  <div class="chat-header">
    {#if inChatView}
      <button class="back-btn" onclick={backToList} title="Back to conversations">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="15 18 9 12 15 6"></polyline>
        </svg>
      </button>
    {/if}
    <span class="header-title">Chat</span>
    <button class="close-btn" onclick={onClose} title="Close chat">
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <line x1="18" y1="6" x2="6" y2="18"></line>
        <line x1="6" y1="6" x2="18" y2="18"></line>
      </svg>
    </button>
  </div>

  {#if !inChatView}
    <ConversationList
      {conversations}
      activeId={activeConversationId}
      onSelect={selectConversation}
      onNew={startNewChat}
      onDelete={deleteConversation}
    />
    <ChatInput onSend={sendMessage} disabled={isStreaming} />
  {:else}
    <MessageList {messages} {streamingContent} />
    <ChatInput onSend={sendMessage} disabled={isStreaming} />
  {/if}
</div>

<style>
  .chat-panel {
    position: fixed;
    right: 0;
    top: 0;
    height: 100%;
    width: 420px;
    max-width: 100vw;
    background: var(--bg-surface);
    border-left: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    z-index: 10;
    transform: translateX(100%);
    transition: transform 0.25s ease;
  }

  .chat-panel.open {
    transform: translateX(0);
  }

  .chat-header {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 12px 16px;
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
  }

  .header-title {
    flex: 1;
    font-size: 14px;
    font-weight: 500;
  }

  .back-btn,
  .close-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    border: none;
    border-radius: 6px;
    background: none;
    color: var(--text-dim);
    cursor: pointer;
  }

  .back-btn:hover,
  .close-btn:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
</style>
