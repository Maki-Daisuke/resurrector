<script lang="ts">
  import { onMount } from "svelte";
  import { EventsOn } from "../wailsjs/runtime/runtime";
  import AgGridSvelte from "ag-grid-svelte";
  import type { ColDef } from "ag-grid-community";
  import "ag-grid-community/styles/ag-grid.css";
  import "ag-grid-community/styles/ag-theme-alpine.css";

  type AppStateInfo = {
    name: string;
    pid: number;
    state: string;
    enabled: boolean;
    command: string;
    args: string[];
    restartCount: number;
  };

  let apps: Record<string, AppStateInfo> = {};

  onMount(() => {
    EventsOn("app_state_update", (dataStr: string) => {
      try {
        const data: AppStateInfo = JSON.parse(dataStr);
        if (data.state === "Removed") {
          delete apps[data.name];
          apps = { ...apps };
        } else {
          apps[data.name] = data;
          apps = { ...apps };
        }
      } catch (e) {
        console.error("Failed to parse event:", e);
      }
    });
  });

  $: appList = Object.values(apps);

  function stateRenderer(params: any) {
    if (!params.value) return '';
    let color = 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300';
    const state = params.value.toLowerCase();
    switch (state) {
      case 'running': color = 'bg-green-100 text-green-800 dark:bg-green-200 dark:text-green-900'; break;
      case 'stopped': color = 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300'; break;
      case 'retrying': color = 'bg-yellow-100 text-yellow-800 dark:bg-yellow-200 dark:text-yellow-900'; break;
      case 'failed': color = 'bg-red-100 text-red-800 dark:bg-red-200 dark:text-red-900'; break;
    }
    return `<span class="px-2.5 py-0.5 rounded text-xs font-semibold ${color}">${params.value}</span>`;
  }

  function enabledRenderer(params: any) {
    if (params.value === undefined) return '';
    const isEnabled = params.value;
    const color = isEnabled ? 'bg-blue-100 text-blue-800 dark:bg-blue-200 dark:text-blue-900' : 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300';
    const text = isEnabled ? 'Enabled' : 'Disabled';
    return `<span class="px-2.5 py-0.5 rounded text-xs font-semibold ${color}">${text}</span>`;
  }

  function commandRenderer(params: any) {
    const data = params.data;
    if (!data) return '';
    return [data.command, ...(data.args || [])].join(' ');
  }

  let columnDefs: ColDef<AppStateInfo>[] = [
    { field: 'name', headerName: 'App Name', flex: 2, minWidth: 150 },
    { field: 'pid', headerName: 'PID', width: 100, valueFormatter: (p: any) => p.value > 0 ? p.value : '-' },
    { field: 'state', headerName: 'State', width: 120, cellRenderer: stateRenderer },
    { field: 'enabled', headerName: 'Enabled/Disabled', width: 150, cellRenderer: enabledRenderer },
    { field: 'command', headerName: 'Command', flex: 3, minWidth: 200, valueGetter: commandRenderer }
  ];

  const defaultColDef: ColDef = {
    resizable: true,
    sortable: true
  };
</script>

<main class="p-6 h-screen w-full flex flex-col items-center box-border bg-gray-50 dark:bg-slate-900">
  <div class="w-full flex-1 shadow-2xl sm:rounded-lg flex flex-col overflow-hidden bg-white dark:bg-gray-800">
    <div class="ag-theme-alpine w-full h-full flex-grow">
      <AgGridSvelte
        rowData={appList}
        {columnDefs}
        {defaultColDef}
        rowSelection="single"
        overlayNoRowsTemplate="<span class='text-gray-500 font-medium'>Waiting for process connection...</span>"
      />
    </div>
  </div>
</main>

<style>
  :global(.ag-theme-alpine) {
    height: 100% !important;
    width: 100% !important;
  }
</style>
