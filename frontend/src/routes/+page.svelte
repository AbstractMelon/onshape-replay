<script lang="ts">
  import { Package, BookOpen, FileText, Download } from 'lucide-svelte';
</script>

<svelte:head>
  <link rel="preconnect" href="https://fonts.googleapis.com" />
  <link
    href="https://fonts.googleapis.com/css2?family=Space+Grotesk:wght@500;700&display=swap"
    rel="stylesheet"
  />
</svelte:head>

<div class="relative flex min-h-screen flex-col bg-[#0e1b2a] text-[#e8f1f8]">
  <div class="pointer-events-none fixed inset-0 bg-[linear-gradient(#4c7290_1px,transparent_1px),linear-gradient(90deg,#4c7290_1px,transparent_1px)] bg-[length:40px_40px] opacity-30"></div>
  <div class="pointer-events-none fixed inset-0 bg-[radial-gradient(ellipse_at_center,rgba(14,27,42,0.15)_0%,rgba(14,27,42,0.97)_65%)]"></div>

  <main class="relative z-10 flex flex-1 flex-col items-center justify-center px-6 py-20 text-center">
    <h1 class="font-display mb-5 text-4xl font-bold leading-tight tracking-tight sm:text-5xl">
        Onshape Replay
    </h1>
    <p class="mb-9 max-w-xl text-lg leading-relaxed text-[#9db6c9]">
      Onshape Replay walks back through your Part Studio's feature tree and turns it into a
      video. Sketch to final part, one feature at a time. Export MP4, GIF, or PNG sequences up
      to 4K.
    </p>

    <div class="flex flex-wrap items-center justify-center gap-3">
      <a
        href="https://cad.onshape.com/appstore/apps/Rendering/6a4b1a514f7102ea7e1d840e"
        class="inline-flex items-center gap-2 rounded-sm bg-[#f2a93b] px-6 py-3 text-sm font-medium text-[#1a1102] transition hover:brightness-110"
      >
        <Download class="h-4 w-4" /> Install from Onshape Store
      </a>
      <a
        href="/docs"
        class="inline-flex items-center gap-2 rounded-sm border border-white/20 px-5 py-3 text-sm font-medium transition hover:border-white/40 hover:bg-white/5"
      >
        <BookOpen class="h-4 w-4" /> Docs
      </a>
      <a
        href="https://github.com/abstractmelon/onshape-replay"
        class="inline-flex items-center gap-2 rounded-sm border border-white/20 px-5 py-3 text-sm font-medium transition hover:border-white/40 hover:bg-white/5"
      >
        <Package class="h-4 w-4" /> GitHub
      </a>
      <a
        href="/eula"
        class="inline-flex items-center gap-2 rounded-sm border border-white/20 px-5 py-3 text-sm font-medium transition hover:border-white/40 hover:bg-white/5"
      >
        <FileText class="h-4 w-4" /> EULA
      </a>
    </div>
    <p class="text-sm text-[#9db6c9] pt-5">Onshape Replay is an independent extension. Not affiliated with Onshape.</p>
  </main>
</div>

<style>
  .font-display {
    font-family: 'Space Grotesk', sans-serif;
  }
</style>
