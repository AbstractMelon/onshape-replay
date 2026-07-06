<script lang="ts">
  import { onMount } from 'svelte';
  import { Loader2, RefreshCw } from 'lucide-svelte';
  import {
    checkAuthStatus,
    redirectToLogin,
    getJobManifest,
    AuthRequiredError
  } from '$lib/api/jobs';
  import type { JobManifest, JobResponse } from '$lib/types/job';
  import ResultPreview from '$lib/components/ResultPreview.svelte';

  let { data } = $props();
  let jobId = $derived(data.jobId);
  let manifest = $state<JobManifest | JobResponse | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let notFound = $state(false);

  async function loadManifest() {
    loading = true;
    error = null;
    notFound = false;

    try {
      const authed = await checkAuthStatus();
      if (!authed) {
        redirectToLogin(window.location.href);
        return;
      }

      const result = await getJobManifest(jobId);
      manifest = result;

      const isFull = 'exportConfig' in result;
      if (!isFull && result.status !== 'completed' && result.status !== 'failed' && result.status !== 'cancelled') {
        // JobResponse fallback - job not yet finished
      }
    } catch (err) {
      if (err instanceof AuthRequiredError) {
        redirectToLogin(window.location.href);
        return;
      }
      if (err instanceof Error && err.message === 'job not found') {
        notFound = true;
      } else {
        error = err instanceof Error ? err.message : String(err);
      }
    } finally {
      loading = false;
    }
  }

  const isFullManifest = $derived(manifest && 'exportConfig' in manifest);
  const jobResponse = $derived(manifest as JobResponse | null);

  onMount(() => {
    loadManifest();
  });
</script>

<div class="mx-auto min-h-screen max-w-5xl bg-white px-4 py-8 sm:px-6 lg:px-8">
  {#if loading}
    <div class="flex items-center justify-center py-32">
      <Loader2 class="h-10 w-10 animate-spin text-blue-600" />
    </div>

  {:else if notFound}
    <div class="flex items-center justify-center py-32">
      <p class="text-center text-gray-500">This render could not be found.</p>
    </div>

  {:else if error}
    <div class="flex flex-col items-center gap-4 py-32">
      <p class="text-center text-red-600">{error}</p>
      <button
        onclick={loadManifest}
        class="flex items-center gap-2 rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
      >
        <RefreshCw class="h-4 w-4" />
        Try again
      </button>
    </div>

  {:else if manifest && isFullManifest}
    <h1 class="mb-8 text-2xl font-bold text-gray-900">Render Result</h1>
    <ResultPreview {manifest} />

  {:else if manifest && jobResponse}
    <!-- JobResponse fallback (in-progress job) -->
    <div class="flex flex-col items-center gap-6 py-16 text-center">
      <Loader2 class="h-12 w-12 animate-spin text-blue-500" />
      <h1 class="text-xl font-semibold text-gray-800">This render is not finished yet.</h1>
      {#if jobResponse.percentComplete != null && jobResponse.percentComplete > 0}
        <p class="text-gray-500">
          {Math.round(jobResponse.percentComplete)}% complete
        </p>
      {/if}
      <p class="max-w-md text-sm text-gray-400">
        The full result will appear here once the render completes.
      </p>
      <button
        onclick={loadManifest}
        class="flex items-center gap-2 rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
      >
        <RefreshCw class="h-4 w-4" />
        Refresh
      </button>
    </div>
  {/if}
</div>
