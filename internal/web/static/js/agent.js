// Agent chat — handles SSE streaming for the agent chat page.
(function() {
  const form = document.getElementById('chat-form');
  if (!form) return;

  const messagesEl = document.getElementById('messages');
  const responseEl = document.getElementById('agent-response');

  form.addEventListener('submit', async function(e) {
    e.preventDefault();
    const fd = new FormData(form);
    const content = fd.get('content');
    if (!content) return;

    // Add user message to DOM
    const userDiv = document.createElement('div');
    userDiv.className = 'flex justify-end';
    userDiv.innerHTML = `<div class="max-w-[70%] rounded-lg bg-primary-600 px-4 py-2 text-sm text-white">${escapeHtml(content)}</div>`;
    messagesEl.appendChild(userDiv);

    // Clear input
    form.querySelector('input[name="content"]').value = '';

    // Create assistant response bubble
    const assistantDiv = document.createElement('div');
    assistantDiv.className = 'flex justify-start';
    const bubbleDiv = document.createElement('div');
    bubbleDiv.className = 'prose prose-sm dark:prose-invert max-w-[70%] rounded-lg bg-gray-100 dark:bg-gray-800 px-4 py-2 text-sm';
    assistantDiv.appendChild(bubbleDiv);
    messagesEl.appendChild(assistantDiv);

    // Scroll to bottom
    messagesEl.scrollTop = messagesEl.scrollHeight;

    // Stream response via fetch + ReadableStream
    try {
      const res = await fetch('/agent/chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          conversationId: fd.get('conversationId'),
          vaultId: fd.get('vault'),
          content: content
        })
      });

      if (!res.ok) {
        bubbleDiv.textContent = 'Error: ' + res.statusText;
        return;
      }

      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';
      let textContent = '';

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop(); // keep incomplete line

        for (const line of lines) {
          if (!line.startsWith('data: ')) continue;
          try {
            const evt = JSON.parse(line.slice(6));
            switch (evt.type) {
              case 'text':
                textContent += evt.content;
                bubbleDiv.innerHTML = textContent;
                messagesEl.scrollTop = messagesEl.scrollHeight;
                break;
              case 'tool_start':
                appendToolCard(messagesEl, evt, 'running');
                break;
              case 'tool_end':
                updateToolCard(evt.callId, 'done', evt.meta);
                break;
              case 'tool_approval_required':
                appendApprovalCard(messagesEl, evt);
                break;
              case 'conv_id':
                // Update hidden field with new conversation ID
                const convInput = form.querySelector('input[name="conversationId"]');
                if (convInput) convInput.value = evt.convId;
                break;
              case 'error':
                bubbleDiv.innerHTML += `<p class="text-red-500">${escapeHtml(evt.content)}</p>`;
                break;
            }
          } catch (e) {
            // ignore parse errors for non-JSON lines
          }
        }
      }
    } catch (err) {
      bubbleDiv.textContent = 'Connection error: ' + err.message;
    }
  });

  function appendToolCard(container, evt, status) {
    const card = document.createElement('div');
    card.id = 'tool-' + evt.callId;
    card.className = 'mx-4 my-2 rounded-md border border-border dark:border-border-dark p-3 text-xs';
    card.innerHTML = `
      <div class="flex items-center gap-2">
        <span class="font-mono font-medium">${escapeHtml(evt.tool)}</span>
        <span class="tool-status rounded-full px-2 py-0.5 bg-accent-400/20 text-accent-500">${status}</span>
      </div>`;
    container.appendChild(card);
  }

  function updateToolCard(callId, status, meta) {
    const card = document.getElementById('tool-' + callId);
    if (!card) return;
    const statusEl = card.querySelector('.tool-status');
    if (statusEl) {
      statusEl.textContent = status;
      statusEl.className = 'tool-status rounded-full px-2 py-0.5 bg-green-100 dark:bg-green-900/20 text-green-700 dark:text-green-400';
    }
  }

  function appendApprovalCard(container, evt) {
    const card = document.createElement('div');
    card.id = 'approval-' + evt.callId;
    card.className = 'mx-4 my-2 rounded-md border-2 border-accent-400 p-3 text-sm';

    const approval = evt.approval || {};
    card.innerHTML = `
      <div class="mb-2 font-medium">Tool requires approval: <span class="font-mono">${escapeHtml(evt.tool)}</span></div>
      <div class="flex gap-2">
        <button onclick="approveAction('${escapeHtml(evt.callId)}', 'approve_all')"
          class="rounded bg-green-600 px-3 py-1 text-xs text-white hover:bg-green-700">Approve</button>
        <button onclick="approveAction('${escapeHtml(evt.callId)}', 'reject')"
          class="rounded bg-red-600 px-3 py-1 text-xs text-white hover:bg-red-700">Reject</button>
      </div>`;
    container.appendChild(card);
  }

  // Global function for approval buttons
  window.approveAction = async function(callId, action) {
    const convId = form.querySelector('input[name="conversationId"]')?.value;
    try {
      await fetch('/agent/approval', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ conversationId: convId, callId: callId, action: action })
      });
      const card = document.getElementById('approval-' + callId);
      if (card) {
        card.innerHTML = `<div class="text-xs text-muted">${action === 'reject' ? 'Rejected' : 'Approved'}</div>`;
      }
    } catch (err) {
      console.error('Approval error:', err);
    }
  };

  function escapeHtml(s) {
    const div = document.createElement('div');
    div.textContent = s;
    return div.innerHTML;
  }
})();
