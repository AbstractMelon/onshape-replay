<script lang="ts">
  import { Download, ExternalLink, Play, CheckCircle, XCircle, AlertCircle, Clock } from 'lucide-svelte';
  import { API_BASE } from '../api/client';
  import type { JobResponse, JobStatus } from '../types/job';
  import ConfirmDialog from './ConfirmDialog.svelte';

  let {
    job,
    onStartNew
  }: {
    job: JobResponse;
    onStartNew: () => void;
  } = $props();

  let showConfirm = $state(false);
  let thumbnailError = $state(false);

  const isFailed = $derived(job.status === 'failed');
  const isCancelled = $derived(job.status === 'cancelled');
  const isCompleted = $derived(job.status === 'completed');

  const thumbnailUrl = $derived(
    !isFailed && !isCancelled
      ? `${API_BASE}/storage/${job.documentId}/${job.elementId}/${job.id}/frames/frame_0001.png`
      : null
  );

  function formatBytes(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  }
</script>

<div class="space-y-4">
  {#if isCompleted}
    <!-- successful result -->
    {#if thumbnailUrl && !thumbnailError}
      <img
        src={thumbnailUrl}
        alt="Render preview"
        class="w-full rounded-lg border border-gray-200 object-cover"
        onerror={() => (thumbnailError = true)}
      />
    {:else}
      <div class="flex aspect-video items-center justify-center rounded-lg bg-gray-100">
        <Play class="h-10 w-10 text-gray-400" />
      </div>
    {/if}

    <div class="flex flex-wrap gap-2">
      <a
        href="/result/{job.id}"
        target="_blank"
        rel="noopener noreferrer"
        class="flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-2 text-sm font-medium text-white hover:bg-blue-700"
      >
        <ExternalLink class="h-4 w-4" />
        Open Result
      </a>

      {#if job.outputs.length === 1}
        <a
          href="{API_BASE}/jobs/{job.id}/download/{job.outputs[0].format}"
          download
          class="flex items-center gap-1.5 rounded-md border border-gray-300 bg-white px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
        >
          <Download class="h-4 w-4" />
          Download {job.outputs[0].format.toUpperCase()}
          <span class="text-xs text-gray-400">({formatBytes(job.outputs[0].size)})</span>
        </a>
      {:else}
        <div class="flex flex-wrap gap-1.5">
          {#each job.outputs as output}
            <a
              href="{API_BASE}/jobs/{job.id}/download/{output.format}"
              download
              class="flex items-center gap-1.5 rounded-md border border-gray-300 bg-white px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
            >
              <Download class="h-4 w-4" />
              {output.format.toUpperCase()}
              <span class="text-xs text-gray-400">({formatBytes(output.size)})</span>
            </a>
          {/each}
        </div>
      {/if}
    </div>

    <div class="flex items-center gap-2 text-sm text-gray-600">
      <CheckCircle class="h-4 w-4 text-green-500" />
      Render complete
    </div>

    <button
      onclick={() => (showConfirm = true)}
      class="w-full rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
    >
      Start new render
    </button>

    <ConfirmDialog
      open={showConfirm}
      title="Start a new render?"
      message="You already have a completed render for this Part Studio. Start a new one anyway?"
      confirmLabel="Start new render"
      cancelLabel="Cancel"
      onConfirm={() => {
        showConfirm = false;
        onStartNew();
      }}
      onCancel={() => (showConfirm = false)}
    />

  {:else if isFailed}
    <div class="rounded-lg border border-red-200 bg-red-50 p-6 text-center">
      <XCircle class="mx-auto h-10 w-10 text-red-400" />
      <h3 class="mt-3 text-lg font-semibold text-red-800">Render failed</h3>
      {#if job.error}
        <p class="mt-2 text-sm text-red-600">{job.error}</p>
      {/if}
    </div>

    <button
      onclick={onStartNew}
      class="w-full rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
    >
      Start new render
    </button>

  {:else if isCancelled}
    <div class="rounded-lg border border-gray-200 bg-gray-50 p-6 text-center">
      <AlertCircle class="mx-auto h-10 w-10 text-gray-400" />
      <h3 class="mt-3 text-lg font-semibold text-gray-700">Render was cancelled</h3>
    </div>

    <button
      onclick={onStartNew}
      class="w-full rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
    >
      Start new render
    </button>
  {/if}
</div>
