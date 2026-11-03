<script lang="ts">
	import SidebarAccount from '$lib/components/app/SidebarAccount.svelte';
	import SidebarAgent from '$lib/components/app/SidebarAgent.svelte';
	import SidebarBrand from '$lib/components/app/SidebarBrand.svelte';
	import SidebarFleet from '$lib/components/app/SidebarFleet.svelte';
	import SidebarNav from '$lib/components/app/SidebarNav.svelte';
	import type { NavGroup, NavItem } from '$lib/utils/nav';

	interface Props {
		/** Icon-only rail: labels collapse and only the icons stay. */
		compact?: boolean;
		/** Fleet overview, which sits above the agent picker and outside its scope. */
		fleet: NavItem;
		groups: NavGroup[];
		/** Dismisses the phone drawer; desktop never shows the close button. */
		onClose: () => void;
	}

	let { compact = false, fleet, groups, onClose }: Props = $props();
</script>

<!--
	Advanced mode is the full navigator: every page of the agent, grouped by
	what it configures, with the fleet overview kept in reach above the picker.
-->
<SidebarBrand {compact} {onClose} />
<SidebarFleet {compact} {fleet} />
<SidebarAgent {compact} />
<SidebarNav {compact} {groups} />
<SidebarAccount {compact} />
