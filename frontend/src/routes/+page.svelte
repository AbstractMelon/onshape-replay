<script lang="ts">
  import { onMount } from 'svelte';
  import { Loader2 } from 'lucide-svelte';

  onMount(() => {
    window.location.href = '/panel';
  });
</script>

<div class="flex min-h-screen items-center justify-center">
  <Loader2 class="h-8 w-8 animate-spin text-blue-600" />
</div>
