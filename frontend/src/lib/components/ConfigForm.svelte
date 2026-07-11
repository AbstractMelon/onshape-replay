<script lang="ts">
  import { Play, Camera } from 'lucide-svelte';
  import type { ExportConfig } from '../types/exportOptions';
  import { DEFAULT_EXPORT_CONFIG } from '../types/exportOptions';

  let {
    initialConfig = DEFAULT_EXPORT_CONFIG,
    onPreview,
    onSubmit,
    submitError
  }: {
    initialConfig?: ExportConfig;
    onPreview?: (config: ExportConfig) => Promise<Blob | null>;
    onSubmit: (config: ExportConfig) => void;
    submitError: string | null;
  } = $props();

  let previewUrl = $state<string | null>(null);
  let previewLoading = $state(false);
  let previewError = $state<string | null>(null);

  async function handlePreview() {
    if (!onPreview) return;
    previewLoading = true;
    previewError = null;
    previewUrl = null;
    try {
      const blob = await onPreview(config);
      if (blob) {
        previewUrl = URL.createObjectURL(blob);
      }
    } catch (err) {
      previewError = err instanceof Error ? err.message : String(err);
    } finally {
      previewLoading = false;
    }
  }

  // Intentionally captures initial prop value once; form maintains its own state
  let config = $state<ExportConfig>({ ...initialConfig });

  let validationError = $state<string | null>(null);

  const resolutions = ['720p', '1080p', '4k'] as const;
  const cameraModes = [
    { value: 'current', label: 'Current camera' },
    { value: 'isometric', label: 'Isometric' },
    { value: 'front', label: 'Front' },
    { value: 'top', label: 'Top' }
  ] as const;

  const bboxModes = [
    { value: 'once', label: 'Once (locked camera)' },
    { value: 'each', label: 'Each frame (re-center)' }
  ] as const;

  const formatOptions = [
    { value: 'mp4', label: 'MP4' },
    { value: 'gif', label: 'GIF' },
    { value: 'png', label: 'PNG sequence' },
    { value: 'zip', label: 'ZIP' }
  ] as const;

  const supportsTransparency = $derived(
    config.formats.includes('png') || config.formats.includes('zip')
  );

  function toggleFormat(format: 'mp4' | 'gif' | 'png' | 'zip') {
    if (config.formats.includes(format)) {
      if (config.formats.length <= 1) return;
      config.formats = config.formats.filter((f) => f !== format);
    } else {
      config.formats = [...config.formats, format];
    }
  }

  function handleSubmit(e: Event) {
    e.preventDefault();
    validationError = null;

    if (config.formats.length === 0) {
      validationError = 'At least one output format must be selected.';
      return;
    }
    if (!Number.isInteger(config.frameRate) || config.frameRate < 1) {
      validationError = 'Frame rate must be a positive integer.';
      return;
    }
    if (
      !Number.isInteger(config.holdFirst) ||
      config.holdFirst < 0 ||
      !Number.isInteger(config.holdLast) ||
      config.holdLast < 0
    ) {
      validationError = 'Hold frames must be non-negative integers.';
      return;
    }

    onSubmit(config);
  }
</script>

<form onsubmit={handleSubmit} class="space-y-5">
  {#if validationError}
    <div class="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-700">
      {validationError}
    </div>
  {/if}

  {#if submitError}
    <div class="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-700">
      {submitError}
    </div>
  {/if}

  <div>
    <span class="mb-1.5 block text-sm font-medium text-gray-700">Resolution</span>
    <div class="flex gap-1 rounded-lg border border-gray-300 bg-gray-50 p-1" role="radiogroup" aria-label="Resolution">
      {#each resolutions as res}
        <button
          type="button"
          onclick={() => (config.resolution = res)}
          class="flex-1 rounded-md px-3 py-1.5 text-sm font-medium transition-colors {config.resolution === res
            ? 'bg-white text-gray-900 shadow-sm'
            : 'text-gray-500 hover:text-gray-700'}"
        >
          {res.toUpperCase()}
        </button>
      {/each}
    </div>
  </div>

  <div>
    <label for="frameRate" class="mb-1.5 block text-sm font-medium text-gray-700">
      Frame rate
    </label>
    <input
      id="frameRate"
      type="number"
      bind:value={config.frameRate}
      min="1"
      max="60"
      class="w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
    />
    <p class="mt-1 text-xs text-gray-500">Between 1 and 60 fps.</p>
  </div>

  <div>
    <label for="cameraMode" class="mb-1.5 block text-sm font-medium text-gray-700">
      Camera mode
    </label>
    <select
      id="cameraMode"
      bind:value={config.cameraMode}
      class="w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
    >
      {#each cameraModes as mode}
        <option value={mode.value}>{mode.label}</option>
      {/each}
    </select>
  </div>

  <div>
    <label for="bboxMode" class="mb-1.5 block text-sm font-medium text-gray-700">
      Bounding box zoom
    </label>
    <select
      id="bboxMode"
      bind:value={config.bboxMode}
      class="w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
    >
      {#each bboxModes as mode}
        <option value={mode.value}>{mode.label}</option>
      {/each}
    </select>
    <p class="mt-1 text-xs text-gray-500">
      "Once" uses the completed model's bounding box for consistent framing. "Each" re-centers on the visible geometry per frame.
    </p>
  </div>

  <div class="flex items-start gap-4">
    <div class="flex-1">
      <label for="bgColor" class="mb-1.5 block text-sm font-medium text-gray-700">
        Background color
      </label>
      <div class="flex gap-2">
        <input
          id="bgColor"
          type="color"
          bind:value={config.bgColor}
          disabled={config.transparent}
          class="h-9 w-12 cursor-pointer rounded border border-gray-300 disabled:opacity-40"
        />
        <input
          type="text"
          bind:value={config.bgColor}
          disabled={config.transparent}
          class="flex-1 rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:bg-gray-100 disabled:text-gray-400"
        />
      </div>
    </div>
    <label class="mt-6 flex items-center gap-2 text-sm">
      <input
        type="checkbox"
        bind:checked={config.transparent}
        class="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
      />
      Transparent
    </label>
  </div>

  {#if config.transparent && !supportsTransparency}
    <p class="text-xs text-amber-600">
      Transparency is only supported for PNG sequence and ZIP output formats.
    </p>
  {/if}

  <div class="flex gap-4">
    <div class="flex-1">
      <label for="holdFirst" class="mb-1.5 block text-sm font-medium text-gray-700">
        Hold first (frames)
      </label>
      <input
        id="holdFirst"
        type="number"
        bind:value={config.holdFirst}
        min="0"
        class="w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      />
    </div>
    <div class="flex-1">
      <label for="holdLast" class="mb-1.5 block text-sm font-medium text-gray-700">
        Hold last (frames)
      </label>
      <input
        id="holdLast"
        type="number"
        bind:value={config.holdLast}
        min="0"
        class="w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
      />
    </div>
  </div>

  <div>
    <label for="fileNaming" class="mb-1.5 block text-sm font-medium text-gray-700">
      File naming pattern
    </label>
    <input
      id="fileNaming"
      type="text"
      bind:value={config.fileNaming}
      class="w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
    />
    <p class="mt-1 text-xs text-gray-500">
      Use <code class="rounded bg-gray-100 px-1">{'{index}'}</code> as a placeholder for the frame number.
    </p>
  </div>

  <div>
    <span class="mb-1.5 block text-sm font-medium text-gray-700">Output formats</span>
    <div class="flex flex-wrap gap-3" role="group" aria-label="Output formats">
      {#each formatOptions as fmt}
        <label class="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={config.formats.includes(fmt.value)}
            onchange={() => toggleFormat(fmt.value)}
            class="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
          />
          {fmt.label}
        </label>
      {/each}
    </div>
  </div>

  <fieldset class="space-y-2 rounded-md border border-gray-200 p-4">
    <legend class="text-sm font-medium text-gray-700">Feature filtering</legend>
    <label class="flex items-center gap-2 text-sm">
      <input
        type="checkbox"
        bind:checked={config.featureLabel}
        class="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
      />
      Overlay feature name on frames
    </label>
    <label class="flex items-center gap-2 text-sm">
      <input
        type="checkbox"
        bind:checked={config.skipSketches}
        disabled={config.geometryOnly}
        class="rounded border-gray-300 text-blue-600 focus:ring-blue-500 disabled:opacity-50"
      />
      Skip sketches
    </label>
    <label class="flex items-center gap-2 text-sm">
      <input
        type="checkbox"
        bind:checked={config.skipSuppressed}
        disabled={config.geometryOnly}
        class="rounded border-gray-300 text-blue-600 focus:ring-blue-500 disabled:opacity-50"
      />
      Skip suppressed features
    </label>
    <label class="flex items-center gap-2 text-sm">
      <input
        type="checkbox"
        bind:checked={config.skipConstruction}
        disabled={config.geometryOnly}
        class="rounded border-gray-300 text-blue-600 focus:ring-blue-500 disabled:opacity-50"
      />
      Skip construction-only features
    </label>
    <hr class="my-2 border-gray-200" />
    <label class="flex items-center gap-2 text-sm">
      <input
        type="checkbox"
        bind:checked={config.geometryOnly}
        class="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
      />
      Only export geometry-changing features
    </label>
  </fieldset>

  <div class="flex gap-2">
    {#if onPreview}
      <button
        type="button"
        onclick={handlePreview}
        disabled={previewLoading}
        class="flex flex-1 items-center justify-center gap-2 rounded-md border border-gray-300 bg-white px-4 py-2.5 text-sm font-medium text-gray-700 hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:cursor-wait disabled:opacity-60"
      >
        <Camera class="h-4 w-4" />
        {previewLoading ? 'Capturing...' : 'Preview'}
      </button>
    {/if}
    <button
      type="submit"
      class="flex flex-1 items-center justify-center gap-2 rounded-md bg-blue-600 px-4 py-2.5 text-sm font-medium text-white hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2"
    >
      <Play class="h-4 w-4" />
      Start render
    </button>
  </div>

  {#if previewUrl}
    <div class="rounded-md border border-gray-200 overflow-hidden">
      <img src={previewUrl} alt="Preview" class="w-full h-auto block" />
    </div>
  {/if}
  {#if previewError}
    <div class="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-700">
      {previewError}
    </div>
  {/if}
</form>
