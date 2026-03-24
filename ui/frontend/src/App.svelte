<script lang="ts">
  import { onMount } from "svelte";
  import { EventsOn } from "../wailsjs/runtime/runtime";
  import { Table, TableBody, TableBodyCell, TableBodyRow, TableHead, TableHeadCell, Badge } from "flowbite-svelte";

  type AppStateInfo = {
    id: number;
    name: string;
    pid: number;
    state: string;
    enabled: boolean;
    command: string;
    restartCount: number;
  };

  let apps: Record<number, AppStateInfo> = {};
  let sortKey: keyof AppStateInfo = 'id';
  let sortAsc = true;

  onMount(() => {
    EventsOn("app_state_update", (dataStr: string) => {
      try {
        const data: AppStateInfo = JSON.parse(dataStr);
        apps[data.id] = data;
        apps = { ...apps };
      } catch (e) {
        console.error("Failed to parse event:", e);
      }
    });
  });

  function handleSort(key: keyof AppStateInfo) {
    if (sortKey === key) {
      sortAsc = !sortAsc;
    } else {
      sortKey = key;
      sortAsc = true;
    }
  }

  $: appList = Object.values(apps).sort((a, b) => {
    let valA = a[sortKey];
    let valB = b[sortKey];
    if (valA < valB) return sortAsc ? -1 : 1;
    if (valA > valB) return sortAsc ? 1 : -1;
    return 0;
  });

  function getStateColor(state: string) {
    switch (state.toLowerCase()) {
      case 'running': return 'green';
      case 'stopped': return 'dark';
      case 'retrying': return 'yellow';
      case 'failed': return 'red';
      default: return 'dark';
    }
  }
</script>

<main class="p-6 h-screen w-full flex flex-col items-center">
  <div class="w-full max-w-6xl shadow-2xl sm:rounded-lg mt-8">
    <Table hoverable={true} class="overflow-hidden sm:rounded-lg">
      <TableHead class="bg-gray-100 dark:bg-gray-800">
        <TableHeadCell on:click={() => handleSort('name')} class="cursor-pointer select-none whitespace-nowrap">
          App Name {sortKey === 'name' ? (sortAsc ? '▲' : '▼') : ''}
        </TableHeadCell>
        <TableHeadCell on:click={() => handleSort('pid')} class="cursor-pointer select-none">
          PID {sortKey === 'pid' ? (sortAsc ? '▲' : '▼') : ''}
        </TableHeadCell>
        <TableHeadCell on:click={() => handleSort('state')} class="cursor-pointer select-none">
          State {sortKey === 'state' ? (sortAsc ? '▲' : '▼') : ''}
        </TableHeadCell>
        <TableHeadCell on:click={() => handleSort('enabled')} class="cursor-pointer select-none">
          Enabled/Disabled {sortKey === 'enabled' ? (sortAsc ? '▲' : '▼') : ''}
        </TableHeadCell>
        <TableHeadCell on:click={() => handleSort('command')} class="cursor-pointer select-none">
          Command {sortKey === 'command' ? (sortAsc ? '▲' : '▼') : ''}
        </TableHeadCell>
      </TableHead>
      <TableBody class="divide-y">
        {#each appList as app}
          <TableBodyRow>
            <TableBodyCell class="font-medium text-gray-900 dark:text-white">
              {app.name}
            </TableBodyCell>
            <TableBodyCell class="font-mono">
              {app.pid > 0 ? app.pid : '-'}
            </TableBodyCell>
            <TableBodyCell>
              <Badge color={getStateColor(app.state)}>{app.state}</Badge>
            </TableBodyCell>
            <TableBodyCell>
              <Badge color={app.enabled ? 'blue' : 'dark'}>{app.enabled ? 'Enabled' : 'Disabled'}</Badge>
            </TableBodyCell>
            <TableBodyCell class="font-mono text-sm max-w-xs truncate" title={app.command}>
              {app.command}
            </TableBodyCell>
          </TableBodyRow>
        {/each}
        {#if appList.length === 0}
          <TableBodyRow>
            <TableBodyCell colspan="5" class="text-center py-12 text-gray-500">
              Waiting for process connection...
            </TableBodyCell>
          </TableBodyRow>
        {/if}
      </TableBody>
    </Table>
  </div>
</main>
