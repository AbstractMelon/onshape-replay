<script lang="ts">
  import { AlertTriangle, X } from 'lucide-svelte';

  let {
    open,
    title,
    message,
    confirmLabel = 'Confirm',
    cancelLabel = 'Cancel',
    onConfirm,
    onCancel
  }: {
    open: boolean;
    title: string;
    message: string;
    confirmLabel?: string;
    cancelLabel?: string;
    onConfirm: () => void;
    onCancel: () => void;
  } = $props();

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      onCancel();
    }
  }

  function handleBackdropClick(e: MouseEvent) {
    if (e.target === e.currentTarget) {
      onCancel();
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

{#if open}
  <!-- svelte-ignore a11y_interactive_supports_focus -->
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
    onclick={handleBackdropClick}
    onkeydown={handleKeydown}
    role="dialog"
    aria-modal="true"
    aria-labelledby="confirm-title"
    tabindex="-1"
  >
    <div class="mx-4 w-full max-w-md rounded-lg bg-white p-6 shadow-xl">
      <div class="flex items-start justify-between">
        <div class="flex items-center gap-3">
          <AlertTriangle class="h-6 w-6 text-amber-500" />
          <h2 id="confirm-title" class="text-lg font-semibold text-gray-900">
            {title}
          </h2>
        </div>
        <button
          onclick={onCancel}
          class="rounded p-1 text-gray-400 hover:text-gray-600"
          aria-label="Close"
        >
          <X class="h-5 w-5" />
        </button>
      </div>
      <p class="mt-4 text-sm text-gray-600">{message}</p>
      <div class="mt-6 flex justify-end gap-3">
        <button
          onclick={onCancel}
          class="rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
        >
          {cancelLabel}
        </button>
        <button
          onclick={onConfirm}
          class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
        >
          {confirmLabel}
        </button>
      </div>
    </div>
  </div>
{/if}
