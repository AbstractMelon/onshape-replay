<script lang="ts">
  import { AlertCircle } from 'lucide-svelte';

  let {
    message,
    onRetry
  }: {
    message: string;
    onRetry?: () => void;
  } = $props();
</script>

<div
  role="alert"
  class="flex items-start gap-3 rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-800"
>
  <AlertCircle class="mt-0.5 h-5 w-5 shrink-0 text-red-500" />
  <div class="flex-1">
    <p class="font-medium">Something went wrong</p>
    <p class="mt-1 text-red-600">{message}</p>
  </div>
  {#if onRetry}
    <button
      onclick={onRetry}
      class="shrink-0 rounded-md border border-red-300 bg-white px-3 py-1.5 text-sm font-medium text-red-700 hover:bg-red-50"
    >
      Retry
    </button>
  {/if}
</div>
