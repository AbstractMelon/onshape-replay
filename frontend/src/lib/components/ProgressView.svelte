<script lang="ts">
  import { Square, Loader2, Wifi, WifiOff } from 'lucide-svelte';
  import { jobStore } from '../stores/jobStore';

  let {
    onCancel,
    onCompleted,
    onFailedOrCancelled
  }: {
    onCancel: () => void;
    onCompleted: () => void;
    onFailedOrCancelled: () => void;
  } = $props();

  let cancelling = $state(false);

  function formatTime(seconds: number): string {
    if (seconds >= 60) {
      const m = Math.floor(seconds / 60);
      const s = seconds % 60;
      return `${m}m ${s}s`;
    }
    return `${seconds}s`;
  }

  function statusLabel(): string {
    const s = $jobStore.job?.status;
    const cs = $jobStore.connectionState;
    if (cs === 'reconnecting') return 'Reconnecting';
    if (cancelling) return 'Cancelling';
    if (s === 'pending') return 'Preparing render';
    if (s === 'running') {
      const j = $jobStore.job!;
      return `Rendering (${j.currentFeatureIndex} of ${j.totalFeatures})`;
    }
    if (s === 'completed') return 'Complete';
    if (s === 'failed') return 'Failed';
    if (s === 'cancelled') return 'Cancelled';
    return 'Preparing...';
  }

  function isActive(): boolean {
    const s = $jobStore.job?.status;
    return s === 'pending' || s === 'running';
  }
</script>

<div class="space-y-4">
  <div class="flex items-center justify-between">
    <div class="flex items-center gap-2">
      {#if $jobStore.connectionState === 'reconnecting'}
        <WifiOff class="h-4 w-4 text-amber-500" />
      {:else}
        <Wifi class="h-4 w-4 text-green-500" />
      {/if}
      <span class="text-sm font-medium">{statusLabel()}</span>
    </div>
    {#if isActive() && !cancelling}
      <button
        onclick={() => {
          cancelling = true;
          onCancel();
        }}
        class="flex items-center gap-1.5 rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50"
      >
        <Square class="h-3.5 w-3.5" />
        Cancel
      </button>
    {:else if cancelling}
      <div class="flex items-center gap-1.5 text-sm text-gray-500">
        <Loader2 class="h-3.5 w-3.5 animate-spin" />
        Cancelling...
      </div>
    {/if}
  </div>

  <div>
    <div class="flex items-center justify-between text-sm text-gray-600">
      <span>
        {$jobStore.job?.currentFeatureName || 'Preparing...'}
      </span>
      <span>{Math.round($jobStore.job?.percentComplete ?? 0)}%</span>
    </div>
    <div class="mt-1.5 h-2 w-full overflow-hidden rounded-full bg-gray-200">
      <div
        class="h-full rounded-full bg-blue-600 transition-all duration-300 ease-out"
        style="width: {Math.round($jobStore.job?.percentComplete ?? 0)}%"
      ></div>
    </div>
  </div>

  {#if $jobStore.job?.estimatedRemainingSeconds != null && $jobStore.job.estimatedRemainingSeconds > 0}
    <p class="text-xs text-gray-500">
      Estimated time remaining: {formatTime($jobStore.job.estimatedRemainingSeconds)}
    </p>
  {/if}

  {#if $jobStore.connectionState === 'reconnecting'}
    <div class="flex items-center gap-2 rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-700">
      <Loader2 class="h-4 w-4 animate-spin" />
      Connection lost, reconnecting...
    </div>
  {/if}
</div>
