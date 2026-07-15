<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { Loader2 } from 'lucide-svelte';
  import {
    checkAuthStatus,
    redirectToLogin,
    getCurrentJob,
    startJob,
    cancelJob,
    previewCapture,
    AuthRequiredError
  } from '$lib/api/jobs';
  import { API_BASE } from '$lib/api/client';
  import { jobStore } from '$lib/stores/jobStore';
  import type { OnshapeContext } from '$lib/onshape/context';
  import type { ExportConfig } from '$lib/types/exportOptions';
  import type { JobResponse, ProgressSnapshot } from '$lib/types/job';
  import ConfigForm from '$lib/components/ConfigForm.svelte';
  import ProgressView from '$lib/components/ProgressView.svelte';
  import ResultCard from '$lib/components/ResultCard.svelte';
  import ErrorBanner from '$lib/components/ErrorBanner.svelte';

  type PanelState =
    | 'loading'
    | 'missingContext'
    | 'noDocument'
    | 'waitingForContext'
    | 'configuring'
    | 'inProgress'
    | 'completed'
    | 'failedOrCancelled'
    | 'error';

  let { data } = $props();
  let initialContext = $derived(data.context);
  let context = $state<OnshapeContext | null>(null);
  let state = $state<PanelState>('loading');
  let errorMessage = $state<string | null>(null);
  let submitError = $state<string | null>(null);
  let urlParams = $derived(
    typeof window !== 'undefined'
      ? Array.from(new URLSearchParams(window.location.search))
      : []
  );
  let currentJob = $state<JobResponse | null>(null);
  let eventSource = $state<EventSource | null>(null);
  let reconnectTimer = $state<ReturnType<typeof setTimeout> | null>(null);
  let contextTimeout = $state<ReturnType<typeof setTimeout> | null>(null);
  let removeMessageListener = $state<(() => void) | null>(null);

  function clearReconnectTimer() {
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
  }

  function handleDone(snapshot: ProgressSnapshot) {
    jobStore.applySnapshot(snapshot);
    currentJob = $jobStore.job;
    if (eventSource) {
      eventSource.close();
      eventSource = null;
    }
    if (snapshot.status === 'completed') {
      state = 'completed';
    } else if (snapshot.status === 'failed' || snapshot.status === 'cancelled') {
      state = 'failedOrCancelled';
    }
  }

  function openEventSource(jobId: string) {
    if (eventSource) {
      eventSource.close();
    }

    const es = new EventSource(`${API_BASE}/jobs/${jobId}/events`, {
      withCredentials: true
    });

    es.addEventListener('progress', (e: MessageEvent) => {
      jobStore.setConnectionState('connected');
      clearReconnectTimer();
      try {
        const snapshot: ProgressSnapshot = JSON.parse(e.data);
        jobStore.applySnapshot(snapshot);
      } catch {
        // Ignore malformed data
      }
    });

    es.addEventListener('done', (e: MessageEvent) => {
      clearReconnectTimer();
      try {
        const snapshot: ProgressSnapshot = JSON.parse(e.data);
        handleDone(snapshot);
      } catch {
        // Ignore malformed data
      }
    });

    es.onerror = () => {
      jobStore.setConnectionState('reconnecting');
      if (!reconnectTimer) {
        reconnectTimer = setTimeout(() => {
          if ($jobStore.connectionState === 'reconnecting') {
            errorMessage = 'Unable to reconnect to the render server. Please reload the panel.';
            state = 'error';
          }
        }, 30000);
      }
    };

    eventSource = es;
  }

  function closeEventSource() {
    clearReconnectTimer();
    if (eventSource) {
      eventSource.close();
      eventSource = null;
    }
  }

  async function handleStartJob(config: ExportConfig) {
    if (!context) return;
    submitError = null;
    try {
      const job = await startJob(context, config);
      currentJob = job;
      jobStore.setJob(job);
      state = 'inProgress';
      openEventSource(job.id);
    } catch (err) {
      if (err instanceof AuthRequiredError) {
        redirectToLogin(window.location.href);
        return;
      }
      submitError = err instanceof Error ? err.message : String(err);
    }
  }

  async function handleCancel() {
    if (!currentJob) return;
    try {
      await cancelJob(currentJob.id);
    } catch (err) {
      if (err instanceof AuthRequiredError) {
        redirectToLogin(window.location.href);
        return;
      }
      if (err instanceof Error && !err.message.includes('already terminated')) {
        console.warn('Cancel error:', err);
      }
    }
  }

  function handleStartNew() {
    state = 'configuring';
    submitError = null;
  }

  async function handleRetry() {
    state = 'loading';
    errorMessage = null;
    await initialize();
  }

  function tryExtractDocumentContext(eventData: unknown, server: string): OnshapeContext | null {
    if (typeof eventData !== 'object' || eventData === null) return null;
    const d = eventData as Record<string, unknown>;
    const docId = typeof d.documentId === 'string' ? d.documentId : undefined;
    const wsId = typeof d.workspaceId === 'string' ? d.workspaceId : undefined;
    const elId = typeof d.elementId === 'string' ? d.elementId : undefined;
    if (docId && wsId && elId) {
      return { documentId: docId, workspaceId: wsId, elementId: elId, server };
    }
    return null;
  }

  async function proceedWithFullContext(ctx: OnshapeContext) {
    if (contextTimeout) {
      clearTimeout(contextTimeout);
      contextTimeout = null;
    }
    context = ctx;
    try {
      const authed = await checkAuthStatus();
      if (!authed) {
        redirectToLogin(window.location.href);
        return;
      }

      const job = await getCurrentJob(ctx);
      if (!job) {
        state = 'configuring';
        return;
      }

      currentJob = job;
      jobStore.setJob(job);

      if (job.status === 'pending' || job.status === 'running') {
        state = 'inProgress';
        openEventSource(job.id);
      } else if (job.status === 'completed') {
        state = 'completed';
      } else {
        state = 'failedOrCancelled';
      }
    } catch (err) {
      if (err instanceof AuthRequiredError) {
        redirectToLogin(window.location.href);
        return;
      }
      state = 'error';
      errorMessage = err instanceof Error ? err.message : String(err);
    }
  }

  async function initialize() {
    context = initialContext;

    if (!context) {
      state = 'missingContext';
      return;
    }

    if (context.server) {
      window.parent.postMessage(
        { documentId: context.documentId, workspaceId: context.workspaceId, elementId: context.elementId, messageName: 'applicationInit' },
        context.server
      );
    }

    if (!context.documentId || !context.workspaceId || !context.elementId) {
      state = 'waitingForContext';

      const server = context.server;

      const listener = (event: MessageEvent) => {
        if (event.origin !== server) return;
        const fullCtx = tryExtractDocumentContext(event.data, server);
        if (fullCtx) {
          if (removeMessageListener) removeMessageListener();
          proceedWithFullContext(fullCtx);
        }
      };

      window.addEventListener('message', listener);
      removeMessageListener = () => window.removeEventListener('message', listener);

      contextTimeout = setTimeout(() => {
        if (removeMessageListener) {
          removeMessageListener();
          removeMessageListener = null;
        }
        const merged = tryExtractDocumentContext(
          Object.fromEntries(urlParams),
          context.server
        );
        if (merged) {
          proceedWithFullContext(merged);
        } else {
          state = 'noDocument';
        }
      }, 10000);

      return;
    }

    try {
      const authed = await checkAuthStatus();
      if (!authed) {
        redirectToLogin(window.location.href);
        return;
      }

      const job = await getCurrentJob(context);
      if (!job) {
        state = 'configuring';
        return;
      }

      currentJob = job;
      jobStore.setJob(job);

      if (job.status === 'pending' || job.status === 'running') {
        state = 'inProgress';
        openEventSource(job.id);
      } else if (job.status === 'completed') {
        state = 'completed';
      } else {
        state = 'failedOrCancelled';
      }
    } catch (err) {
      if (err instanceof AuthRequiredError) {
        redirectToLogin(window.location.href);
        return;
      }
      state = 'error';
      errorMessage = err instanceof Error ? err.message : String(err);
    }
  }

  onMount(() => {
    initialize();
  });

  onDestroy(() => {
    closeEventSource();
    if (removeMessageListener) removeMessageListener();
    if (contextTimeout) clearTimeout(contextTimeout);
  });
</script>

<div class="min-h-screen bg-white p-4">
  {#if state === 'loading' || state === 'waitingForContext'}
    <div class="flex items-center justify-center py-20">
      <Loader2 class="h-8 w-8 animate-spin text-blue-600" />
    </div>

  {:else if state === 'missingContext'}
    <div class="flex items-center justify-center py-20">
      <p class="text-center text-sm text-gray-500">
        This page must be opened from within Onshape.
      </p>
    </div>

  {:else if state === 'noDocument'}
    <div class="flex flex-col items-center justify-center py-20">
      <p class="text-center text-sm text-gray-500">
        Open a Part Studio and then open this extension from the right panel to start recording.
      </p>
      <details class="mt-6 w-full max-w-xs">
        <summary class="cursor-pointer text-xs text-gray-400 hover:text-gray-600">Debug: URL params</summary>
        <div class="mt-2 overflow-auto rounded bg-gray-100 p-3 text-xs text-gray-600">
          {#each urlParams as [k, v]}
            <div>{k}={v}</div>
          {/each}
        </div>
      </details>
    </div>

  {:else if state === 'configuring'}
    <ConfigForm
      {context}
      onSubmit={handleStartJob}
      onPreview={async (cfg) => {
        if (!context) return null;
        try {
          return await previewCapture(context, cfg);
        } catch (err) {
          if (err instanceof AuthRequiredError) {
            redirectToLogin(window.location.href);
            return null;
          }
          throw err;
        }
      }}
      {submitError}
    />

  {:else if state === 'inProgress'}
    <ProgressView
      onCancel={handleCancel}
      onCompleted={() => (state = 'completed')}
      onFailedOrCancelled={() => (state = 'failedOrCancelled')}
    />

  {:else if state === 'completed' && currentJob}
    <ResultCard job={currentJob} onStartNew={handleStartNew} />

  {:else if state === 'failedOrCancelled' && currentJob}
    <ResultCard job={currentJob} onStartNew={handleStartNew} />

  {:else if state === 'error'}
    <ErrorBanner message={errorMessage ?? 'Something went wrong, please try again.'} onRetry={handleRetry} />
  {/if}
</div>
