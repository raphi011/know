<script lang="ts">
  let { status }: { status: 'idle' | 'saving' | 'saved' | 'error' } = $props()
</script>

{#if status !== 'idle'}
  <div class="save-status" class:saving={status === 'saving'} class:saved={status === 'saved'} class:error={status === 'error'}>
    {#if status === 'saving'}
      Saving...
    {:else if status === 'saved'}
      Saved
    {:else if status === 'error'}
      Save failed
    {/if}
  </div>
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
