<script lang="ts">
  import { onMount } from 'svelte'
  import { EditorView, keymap } from '@codemirror/view'
  import { EditorState } from '@codemirror/state'
  import { markdown } from '@codemirror/lang-markdown'
  import { oneDark } from '@codemirror/theme-one-dark'
  import { defaultKeymap, history, historyKeymap } from '@codemirror/commands'

  let {
    content,
    onChange,
    onSave,
  }: {
    content: string
    onChange: (content: string) => void
    onSave: () => void
  } = $props()

  let container: HTMLDivElement
  let view: EditorView | undefined

  onMount(() => {
    view = new EditorView({
      parent: container,
      state: createState(content),
    })

    return () => view?.destroy()
  })

  function createState(doc: string) {
    return EditorState.create({
      doc,
      extensions: [
        markdown(),
        oneDark,
        EditorView.lineWrapping,
        history(),
        keymap.of([
          ...defaultKeymap,
          ...historyKeymap,
          {
            key: 'Mod-s',
            run: () => {
              onSave()
              return true
            },
          },
        ]),
        EditorView.updateListener.of((update) => {
          if (update.docChanged) {
            onChange(update.state.doc.toString())
          }
        }),
      ],
    })
  }

  // Replace document when content prop changes (entity switch)
  $effect(() => {
    if (view && view.state.doc.toString() !== content) {
      view.setState(createState(content))
    }
  })
</script>

<div class="editor-container" bind:this={container}></div>

<style>
  .editor-container {
    flex: 1;
    overflow: hidden;
  }

  .editor-container :global(.cm-editor) {
    height: 100%;
  }

  .editor-container :global(.cm-scroller) {
    overflow: auto;
  }
</style>
