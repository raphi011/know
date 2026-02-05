<script lang="ts">
  let { status }: { status: 'idle' | 'saving' | 'saved' | 'error' } = $props()
</script>

{#if status !== 'idle'}
  <span class="save-status {status}">
    {#if status === 'saving'}
      Saving...
    {:else if status === 'saved'}
      Saved
    {:else}
      Save failed
    {/if}
  </span>
{/if}

<style>
  .save-status {
    padding: 4px 12px;
    font-size: 12px;
    border-radius: 4px;
  }

  .saving {
    color: var(--warning);
  }

  .saved {
    color: var(--success);
  }

  .error {
    color: var(--error);
  }
</style>
