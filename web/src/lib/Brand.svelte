<script lang="ts">
	// The app's name with its lamp-lit pause mark, as the header and the welcome show it.
	import Icon from '$lib/Icon.svelte';
	import { strings } from '$lib/strings';
</script>

<Icon name="pause" class="size-6 text-lamp" />
<span class="font-display text-xl font-bold">{strings.appName}</span>
