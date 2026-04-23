<script lang="ts">
  import { onDestroy, onMount } from "svelte";
  import {
    createGrid,
    type ColDef,
    type GridApi,
    type GridOptions,
    type RowDoubleClickedEvent,
  } from "ag-grid-community";

  // ---------------------------------------------------------------------------
  // Props
  // ---------------------------------------------------------------------------
  type T = any;
  export let rowData: T[] = [];
  export let columnDefs: ColDef<T>[] = [];
  export let defaultColDef: ColDef<T> | undefined = undefined;
  export let rowSelection: GridOptions<T>["rowSelection"] = undefined;
  export let overlayNoRowsTemplate: string | undefined = undefined;
  export let onRowDoubleClicked:
    | ((event: RowDoubleClickedEvent<T>) => void)
    | undefined = undefined;

  // ---------------------------------------------------------------------------
  // Internal state
  // ---------------------------------------------------------------------------
  let containerEl: HTMLDivElement;
  let api: GridApi<T> | undefined;

  onMount(() => {
    const options: GridOptions<T> = {
      rowData,
      columnDefs,
      defaultColDef,
      rowSelection,
      overlayNoRowsTemplate,
      onRowDoubleClicked,
    };
    api = createGrid(containerEl, options);
  });

  onDestroy(() => {
    api?.destroy();
    api = undefined;
  });

  // ---------------------------------------------------------------------------
  // Reactive prop sync (after initial mount)
  // ---------------------------------------------------------------------------
  $: if (api) api.setGridOption("rowData", rowData);
  $: if (api) api.setGridOption("columnDefs", columnDefs);
  $: if (api && defaultColDef !== undefined)
    api.setGridOption("defaultColDef", defaultColDef);
</script>

<div bind:this={containerEl} class="ag-grid-wrapper"></div>

<style>
  .ag-grid-wrapper {
    width: 100%;
    height: 100%;
  }
</style>
