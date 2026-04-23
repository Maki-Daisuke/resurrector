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
  interface Props {
    rowData?: T[];
    columnDefs?: ColDef<T>[];
    defaultColDef?: ColDef<T> | undefined;
    rowSelection?: GridOptions<T>["rowSelection"];
    overlayNoRowsTemplate?: string | undefined;
    onRowDoubleClicked?:
      | ((event: RowDoubleClickedEvent<T>) => void)
      | undefined;
  }

  let {
    rowData = [],
    columnDefs = [],
    defaultColDef = undefined,
    rowSelection = undefined,
    overlayNoRowsTemplate = undefined,
    onRowDoubleClicked = undefined,
  }: Props = $props();

  // ---------------------------------------------------------------------------
  // Internal state
  // ---------------------------------------------------------------------------
  let containerEl: HTMLDivElement | undefined = $state();
  let api: GridApi<T> | undefined = $state();

  onMount(() => {
    const options: GridOptions<T> = {
      rowData,
      columnDefs,
      defaultColDef,
      rowSelection,
      overlayNoRowsTemplate,
      onRowDoubleClicked,
    };
    api = createGrid(containerEl!, options);
  });

  onDestroy(() => {
    api?.destroy();
    api = undefined;
  });

  // ---------------------------------------------------------------------------
  // Reactive prop sync (after initial mount)
  // ---------------------------------------------------------------------------
  $effect(() => {
    if (api) api.setGridOption("rowData", rowData);
  });
  $effect(() => {
    if (api) api.setGridOption("columnDefs", columnDefs);
  });
  $effect(() => {
    if (api && defaultColDef !== undefined)
      api.setGridOption("defaultColDef", defaultColDef);
  });
</script>

<div bind:this={containerEl} class="ag-grid-wrapper"></div>

<style>
  .ag-grid-wrapper {
    width: 100%;
    height: 100%;
  }
</style>
