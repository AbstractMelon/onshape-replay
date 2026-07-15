<script lang="ts">
  import { Download, ExternalLink, Video, Image, FileArchive } from 'lucide-svelte';
  import { API_BASE } from '../api/client';
  import type { JobManifest, JobResponse, OutputFile } from '../types/job';
  import type { ExportConfig } from '../types/exportOptions';

  let {
    manifest
  }: {
    manifest: JobManifest | JobResponse;
  } = $props();

  const isFullManifest = $derived('exportConfig' in manifest && 'features' in manifest);
  const fullManifest = $derived(isFullManifest ? (manifest as JobManifest) : null);
  const jobId = $derived(manifest.jobId || (manifest as JobResponse).id);

  const outputs = $derived(manifest.outputs ?? []);
  const previewOutput = $derived(outputs[0]);

  function formatBytes(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  }

  function formatTimestamp(ts: string | null | undefined): string {
    if (!ts) return '--';
    return new Date(ts).toLocaleString();
  }
</script>

<div class="space-y-8">
  <!-- preview area -->
  {#if previewOutput}
    <div class="overflow-hidden rounded-xl border border-gray-200 bg-black">
      {#if previewOutput.format === 'mp4'}
        <video
          controls
          class="w-full"
          src="{API_BASE}/jobs/{jobId}/download/{previewOutput.format}"
        >
          <track kind="captions" src="" label="No captions" />
          Your browser does not support the video element.
        </video>
      {:else if previewOutput.format === 'gif'}
        <img
          src="{API_BASE}/jobs/{jobId}/download/{previewOutput.format}"
          alt="Render result"
          class="w-full"
        />
      {:else}
        <div class="flex aspect-video items-center justify-center bg-gray-900 text-gray-400">
          <FileArchive class="h-16 w-16" />
          <span class="ml-3 text-lg">Archive output (no inline preview)</span>
        </div>
      {/if}
    </div>

  {/if}

  <!-- download buttons -->
  <div class="flex flex-wrap gap-3">
    {#each outputs as output}
      <a
        href="{API_BASE}/jobs/{jobId}/download/{output.format}"
        download
        class="flex items-center gap-2 rounded-lg border border-gray-300 bg-white px-4 py-3 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50"
      >
        {#if output.format === 'mp4'}
          <Video class="h-5 w-5 text-blue-500" />
        {:else if output.format === 'gif'}
          <Image class="h-5 w-5 text-green-500" />
        {:else}
          <Download class="h-5 w-5 text-gray-500" />
        {/if}
        <div class="text-left">
          <div>Download {output.format.toUpperCase()}</div>
          <div class="text-xs text-gray-400">{formatBytes(output.size)}</div>
        </div>
      </a>
    {/each}
  </div>

  <!-- config metadata (only for full manifest) -->
  {#if fullManifest && fullManifest.exportConfig}
    {@const ec = fullManifest.exportConfig as ExportConfig}

    <div class="rounded-lg border border-gray-200 bg-gray-50 p-4">
      <h3 class="mb-3 text-sm font-semibold text-gray-700">Export configuration</h3>
      <dl class="grid grid-cols-2 gap-x-6 gap-y-2 text-sm">
        <div class="text-gray-500">Resolution</div>
        <div>{ec.resolution}</div>
        <div class="text-gray-500">Frame rate</div>
        <div>{ec.frameRate} fps</div>
        <div class="text-gray-500">Camera mode</div>
        <div class="capitalize">{ec.viewMatrix ? `Named view: ${ec.cameraMode}` : ec.cameraMode}</div>
        <div class="text-gray-500">Zoom</div>
        <div>{ec.zoom.toFixed(1)}x</div>
        <div class="text-gray-500">Background</div>
        <div>{ec.transparent ? 'Transparent' : ec.bgColor}</div>
        {#if ec.skipSketches}<div class="text-gray-500">Skip sketches</div><div>Yes</div>{/if}
        {#if ec.skipSuppressed}<div class="text-gray-500">Skip suppressed</div><div>Yes</div>{/if}
        {#if ec.skipConstruction}<div class="text-gray-500">Skip construction</div><div>Yes</div>{/if}
        {#if ec.geometryOnly}<div class="text-gray-500">Geometry only</div><div>Yes</div>{/if}
      </dl>
    </div>

    {#if fullManifest.features}
      <div class="rounded-lg border border-gray-200 bg-gray-50 p-4">
        <h3 class="mb-3 text-sm font-semibold text-gray-700">
          Features ({fullManifest.features.length})
        </h3>
        <div class="text-xs text-gray-500">
          <p>Created: {formatTimestamp(fullManifest.createdAt)}</p>
          <p>Started: {formatTimestamp(fullManifest.startedAt)}</p>
          <p>Completed: {formatTimestamp(fullManifest.completedAt)}</p>
        </div>
      </div>
    {/if}
  {/if}
</div>
