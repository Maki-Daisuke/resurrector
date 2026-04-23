<script lang="ts">
  import { onDestroy, onMount } from "svelte";
  import {
    EventsOn,
    OnFileDrop,
    OnFileDropOff,
  } from "../wailsjs/runtime/runtime";
  import {
    GetFullConfig,
    UpdateAppConfig,
    DeleteAppConfig,
  } from "../wailsjs/go/main/App";
  import type { main } from "../wailsjs/go/models";
  import AgGridSvelte from "./AgGrid.svelte";
  import type { ColDef } from "ag-grid-community";
  import "ag-grid-community/styles/ag-grid.css";
  import "ag-grid-community/styles/ag-theme-alpine.css";

  // ---------------------------------------------------------------------------
  // Types
  // ---------------------------------------------------------------------------
  type AppStateInfo = {
    name: string;
    pid: number;
    state: string;
    enabled: boolean;
    command: string;
    args: string[];
    restartCount: number;
  };

  // ---------------------------------------------------------------------------
  // State: process monitor table
  // ---------------------------------------------------------------------------
  let apps: Record<string, AppStateInfo> = $state({});

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

    OnFileDrop((_x: number, _y: number, paths: string[]) => {
      handleDroppedCommand(paths);
    }, false);
  });

  onDestroy(() => {
    OnFileDropOff();
  });

  let appList = $derived(Object.values(apps));

  // ---------------------------------------------------------------------------
  // Cell renderers
  // ---------------------------------------------------------------------------
  function stateRenderer(params: any) {
    if (!params.value) return "";
    let color = "bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300";
    const state = params.value.toLowerCase();
    switch (state) {
      case "running":
        color =
          "bg-green-100 text-green-800 dark:bg-green-200 dark:text-green-900";
        break;
      case "stopped":
        color = "bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300";
        break;
      case "retrying":
        color =
          "bg-yellow-100 text-yellow-800 dark:bg-yellow-200 dark:text-yellow-900";
        break;
      case "failed":
        color = "bg-red-100 text-red-800 dark:bg-red-200 dark:text-red-900";
        break;
    }
    return `<span class="px-2.5 py-0.5 rounded-sm text-xs font-semibold ${color}">${params.value}</span>`;
  }

  function enabledRenderer(params: any) {
    if (params.value === undefined) return "";
    const isEnabled = params.value;
    const color = isEnabled
      ? "bg-blue-100 text-blue-800 dark:bg-blue-200 dark:text-blue-900"
      : "bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300";
    return `<span class="px-2.5 py-0.5 rounded-sm text-xs font-semibold ${color}">${isEnabled ? "Enabled" : "Disabled"}</span>`;
  }

  function commandRenderer(params: any) {
    const data = params.data;
    if (!data) return "";
    return [data.command, ...(data.args || [])].join(" ");
  }

  let columnDefs: ColDef<AppStateInfo>[] = [
    { field: "name", headerName: "App Name", flex: 2, minWidth: 150 },
    {
      field: "pid",
      headerName: "PID",
      width: 100,
      valueFormatter: (p: any) => (p.value > 0 ? p.value : "-"),
    },
    {
      field: "state",
      headerName: "State",
      width: 120,
      cellRenderer: stateRenderer,
    },
    {
      field: "enabled",
      headerName: "Enabled",
      width: 130,
      cellRenderer: enabledRenderer,
    },
    {
      field: "command",
      headerName: "Command",
      flex: 3,
      minWidth: 200,
      valueGetter: commandRenderer,
    },
  ];

  const defaultColDef: ColDef = { resizable: true, sortable: true };

  // ---------------------------------------------------------------------------
  // Edit Dialog State
  // ---------------------------------------------------------------------------
  let dialogOpen = $state(false);
  let confirmDelete = $state(false);
  let isSaving = $state(false);
  let isPickingCommand = $state(false);
  let isPickingStopCommand = $state(false);
  let errorMessage = $state("");
  let isCreateMode = $state(false);

  let commandDropExtensions = [".exe", ".cmd", ".bat", ".com", ".ps1", ".vbs"];
  (async () => {
    commandDropExtensions = await getCommandExtensions();
  })();

  let editingOriginalName = $state("");

  let editForm: main.AppConfig = $state(makeEmptyForm());

  function makeEmptyForm(): main.AppConfig {
    return {
      name: "",
      enabled: true,
      command: "",
      args: "",
      stopCommand: "",
      stopArgs: "",
      cwd: "",
      restartDelaySec: 0,
      healthyTimeoutSec: 0,
      hideWindow: false,
      maxRetries: -1,
      stopTimeoutSec: 5,
    } as main.AppConfig;
  }

  async function openEditDialog(appName: string) {
    errorMessage = "";
    confirmDelete = false;
    isSaving = false;
    isCreateMode = false;
    try {
      const configs = await GetFullConfig();
      const cfg = configs[appName];
      if (!cfg) {
        const msg = `App "${appName}" not found in config.`;
        errorMessage = msg;
        alert(msg);
        return;
      }
      editingOriginalName = appName;
      editForm = { ...cfg };
      dialogOpen = true;
    } catch (e: any) {
      const msg = `Failed to load config: ${e}`;
      errorMessage = msg;
      alert(msg);
    }
  }

  function openCreateDialog() {
    errorMessage = "";
    confirmDelete = false;
    isSaving = false;
    isCreateMode = true;
    editingOriginalName = "";
    editForm = makeEmptyForm();
    dialogOpen = true;
  }

  function handleRowDoubleClick(event: any) {
    // Ag-grid event data is typically in event.data
    const name: string =
      event?.data?.name ||
      event?.detail?.data?.name ||
      (event?.node && event.node.data && event.node.data.name);

    if (name) {
      openEditDialog(name);
    } else {
      console.error("Could not determine app name from event", event);
    }
  }

  function closeDialog() {
    dialogOpen = false;
    confirmDelete = false;
    errorMessage = "";
    isCreateMode = false;
  }

  function getCommandExtensions(): Promise<string[]> {
    return (window as any).go.main.App.GetCommandExtensions();
  }

  function setCommandValue(command: string) {
    editForm = { ...editForm, command };
  }

  function isSupportedCommandFile(path: string): boolean {
    const normalized = path.trim().toLowerCase();
    return commandDropExtensions.some((ext) => normalized.endsWith(ext));
  }

  function handleDroppedCommand(paths: string[]) {
    if (!paths || paths.length === 0) {
      return;
    }

    const droppedPath = (paths[0] || "").trim();
    if (!droppedPath) {
      return;
    }

    if (!isSupportedCommandFile(droppedPath)) {
      errorMessage =
        "Dropped file is not a supported command type. Drop an executable or script file.";
      return;
    }

    errorMessage = "";

    if (dialogOpen) {
      confirmDelete = false;
      setCommandValue(droppedPath);
      return;
    }

    openCreateDialog();
    setCommandValue(droppedPath);
  }

  function selectCommandPath(current: string): Promise<string> {
    return (window as any).go.main.App.SelectCommandPath(current);
  }

  async function handleBrowseCommand() {
    errorMessage = "";
    isPickingCommand = true;
    try {
      const selected = await selectCommandPath(editForm.command || "");
      if (selected && selected.trim()) {
        setCommandValue(selected.trim());
      }
    } catch (e: any) {
      errorMessage = `Command selection failed: ${e}`;
    } finally {
      isPickingCommand = false;
    }
  }

  async function handleBrowseStopCommand() {
    errorMessage = "";
    isPickingStopCommand = true;
    try {
      const selected = await selectCommandPath(editForm.stopCommand || "");
      if (selected && selected.trim()) {
        editForm = { ...editForm, stopCommand: selected.trim() };
      }
    } catch (e: any) {
      errorMessage = `Stop command selection failed: ${e}`;
    } finally {
      isPickingStopCommand = false;
    }
  }

  async function handleSave() {
    const targetName = editForm.name.trim();
    const targetCommand = editForm.command.trim();

    if (!targetName) {
      errorMessage = "App Name is required.";
      return;
    }
    if (!targetCommand) {
      errorMessage = "Command is required.";
      return;
    }
    isSaving = true;
    errorMessage = "";
    try {
      if (isCreateMode) {
        const configs = await GetFullConfig();
        if (configs[targetName]) {
          errorMessage = `App "${targetName}" already exists.`;
          return;
        }
      }

      await UpdateAppConfig(editingOriginalName, {
        ...editForm,
        name: targetName,
        command: targetCommand,
      });
      closeDialog();
    } catch (e: any) {
      errorMessage = `Save failed: ${e}`;
    } finally {
      isSaving = false;
    }
  }

  function handleDeleteClick() {
    confirmDelete = true;
  }

  async function handleDeleteConfirm() {
    isSaving = true;
    errorMessage = "";
    try {
      await DeleteAppConfig(editingOriginalName);
      closeDialog();
    } catch (e: any) {
      errorMessage = `Delete failed: ${e}`;
    } finally {
      isSaving = false;
    }
  }

  function handleDeleteCancel() {
    confirmDelete = false;
  }

  function clampInt(value: number, min: number, max: number): number {
    return Math.min(max, Math.max(min, Math.round(value)));
  }
</script>

<!-- =========================================================================
     Main Layout
     ========================================================================= -->
<main
  class="p-6 h-screen w-full flex flex-col items-center box-border bg-gray-50 dark:bg-slate-900"
>
  <div
    class="w-full flex-1 shadow-2xl sm:rounded-lg flex flex-col overflow-hidden bg-white dark:bg-gray-800"
  >
    <div class="ag-theme-alpine w-full h-full grow">
      <AgGridSvelte
        rowData={appList}
        {columnDefs}
        {defaultColDef}
        rowSelection="single"
        onRowDoubleClicked={handleRowDoubleClick}
        overlayNoRowsTemplate="<span class='text-gray-500 font-medium'>Waiting for process connection...</span>"
      />
    </div>
    <div class="toolbar">
      <button class="btn btn-primary" type="button" onclick={openCreateDialog}
        >Add</button
      >
    </div>
    <div class="drop-hint">
      Drop an executable onto the window to register a new app entry.
    </div>
  </div>
</main>

<!-- =========================================================================
     Edit Dialog
     ========================================================================= -->
{#if dialogOpen}
  <!-- Backdrop -->
  <div class="dialog-backdrop" onclick={closeDialog} role="presentation"></div>

  <!-- Modal -->
  <div
    class="dialog-panel"
    role="dialog"
    aria-modal="true"
    aria-labelledby="dialog-title"
  >
    <div class="dialog-header">
      <h2 id="dialog-title" class="dialog-title">
        {isCreateMode ? "Add App Configuration" : "Edit App Configuration"}
      </h2>
    </div>

    {#if confirmDelete}
      <!-- ── Delete Confirmation ───────────────────────────────────────── -->
      <div class="confirm-delete-area">
        <div class="confirm-icon">⚠️</div>
        <p class="confirm-text">
          Are you sure you want to delete <strong
            >"{editingOriginalName}"</strong
          >?<br />
          This cannot be undone.
        </p>
        <div class="confirm-buttons">
          <button
            class="btn btn-ghost"
            onclick={handleDeleteCancel}
            disabled={isSaving}
          >
            Cancel
          </button>
          <button
            class="btn btn-danger"
            onclick={handleDeleteConfirm}
            disabled={isSaving}
          >
            {isSaving ? "Deleting…" : "Yes, Delete"}
          </button>
        </div>
      </div>
    {:else}
      <!-- ── Form ──────────────────────────────────────────────────────── -->
      <form
        class="dialog-form"
        onsubmit={(e) => {
          e.preventDefault();
          handleSave();
        }}
      >
        <!-- App Name -->
        <div class="field field-full">
          <label class="field-label" for="field-name"
            >App Name <span class="required">*</span></label
          >
          <input
            id="field-name"
            class="field-input"
            type="text"
            bind:value={editForm.name}
            placeholder="My Application"
          />
        </div>

        <!-- Command -->
        <div class="field field-full">
          <label class="field-label" for="field-command"
            >Command <span class="required">*</span></label
          >
          <div class="input-with-button">
            <input
              id="field-command"
              class="field-input field-mono"
              type="text"
              bind:value={editForm.command}
              placeholder="C:\\Windows\\System32\\cmd.exe"
            />
            <button
              type="button"
              class="btn btn-ghost btn-compact"
              onclick={handleBrowseCommand}
              disabled={isSaving || isPickingCommand}
            >
              {isPickingCommand ? "Opening..." : "Browse..."}
            </button>
          </div>
        </div>

        <!-- Args -->
        <div class="field field-full">
          <label class="field-label" for="field-args">
            Args
            <span class="field-hint"
              >Shell-style: <code>/c "echo hello" --debug</code></span
            >
          </label>
          <input
            id="field-args"
            class="field-input field-mono"
            type="text"
            bind:value={editForm.args}
            placeholder='/c "My App" --verbose'
          />
        </div>

        <!-- CWD -->
        <div class="field field-full">
          <label class="field-label" for="field-cwd">
            Working Directory
            <span class="field-hint"
              >Optional – defaults to the command's directory</span
            >
          </label>
          <input
            id="field-cwd"
            class="field-input field-mono"
            type="text"
            bind:value={editForm.cwd}
            placeholder="C:\path\to\cwd (optional)"
          />
        </div>

        <!-- Checkboxes: Hide Window / Enabled -->
        <div class="checkbox-row">
          <label class="checkbox-label" for="field-hide-window">
            <input
              id="field-hide-window"
              type="checkbox"
              class="checkbox"
              bind:checked={editForm.hideWindow}
            />
            <span>Hide Window</span>
          </label>
          <label class="checkbox-label" for="field-enabled">
            <input
              id="field-enabled"
              type="checkbox"
              class="checkbox"
              bind:checked={editForm.enabled}
            />
            <span>Enabled</span>
          </label>
        </div>

        <!-- Stop Command -->
        <div class="field field-full">
          <label class="field-label" for="field-stop-command">
            Stop Command
            <span class="field-hint"
              >Optional executable. Leave empty for automatic stop detection.</span
            >
          </label>
          <div class="input-with-button">
            <input
              id="field-stop-command"
              class="field-input field-mono"
              type="text"
              bind:value={editForm.stopCommand}
              placeholder="taskkill"
            />
            <button
              type="button"
              class="btn btn-ghost btn-compact"
              onclick={handleBrowseStopCommand}
              disabled={isSaving || isPickingStopCommand}
            >
              {isPickingStopCommand ? "Opening..." : "Browse..."}
            </button>
          </div>
        </div>

        <!-- Stop Args -->
        <div class="field field-full">
          <label class="field-label" for="field-stop-args">
            Stop Args
            <span class="field-hint"
              >Shell-style argv. <code>{"${PID}"}</code> is replaced with the monitored
              PID.</span
            >
          </label>
          <input
            id="field-stop-args"
            class="field-input field-mono"
            type="text"
            bind:value={editForm.stopArgs}
            placeholder={"/PID ${PID} /T"}
          />
        </div>

        <!-- Numeric fields row: Stop Timeout / Restart Delay -->
        <div class="field-row">
          <div class="field">
            <label class="field-label" for="field-stop-timeout">
              Stop Timeout (s)
              <span class="field-hint">Default: 5</span>
            </label>
            <div class="number-input-wrap">
              <input
                id="field-stop-timeout"
                class="field-input field-number"
                type="number"
                min="0"
                max="3600"
                bind:value={editForm.stopTimeoutSec}
                onchange={() =>
                  (editForm.stopTimeoutSec = clampInt(
                    editForm.stopTimeoutSec,
                    0,
                    3600,
                  ))}
              />
            </div>
          </div>
          <div class="field">
            <label class="field-label" for="field-restart-delay">
              Restart Delay (s)
              <span class="field-hint">Default: 0</span>
            </label>
            <div class="number-input-wrap">
              <input
                id="field-restart-delay"
                class="field-input field-number"
                type="number"
                min="0"
                max="3600"
                bind:value={editForm.restartDelaySec}
                onchange={() =>
                  (editForm.restartDelaySec = clampInt(
                    editForm.restartDelaySec,
                    0,
                    3600,
                  ))}
              />
            </div>
          </div>
        </div>

        <!-- Numeric fields row: Max Retries / Healthy Timeout -->
        <div class="field-row">
          <div class="field">
            <label class="field-label" for="field-max-retries">
              Max Retries
              <span class="field-hint">0 = no retry, &lt;0 = infinite</span>
            </label>
            <div class="number-input-wrap">
              <input
                id="field-max-retries"
                class="field-input field-number"
                type="number"
                min="-1"
                max="999"
                bind:value={editForm.maxRetries}
                onchange={() =>
                  (editForm.maxRetries = clampInt(
                    editForm.maxRetries,
                    -1,
                    999,
                  ))}
              />
            </div>
          </div>
          <div class="field">
            <label class="field-label" for="field-healthy-timeout">
              Healthy Timeout (s)
              <span class="field-hint">0 = no uptime reset</span>
            </label>
            <div class="number-input-wrap">
              <input
                id="field-healthy-timeout"
                class="field-input field-number"
                type="number"
                min="0"
                max="3600"
                bind:value={editForm.healthyTimeoutSec}
                onchange={() =>
                  (editForm.healthyTimeoutSec = clampInt(
                    editForm.healthyTimeoutSec,
                    0,
                    3600,
                  ))}
              />
            </div>
          </div>
        </div>

        <!-- Error message -->
        {#if errorMessage}
          <div class="error-banner" role="alert">{errorMessage}</div>
        {/if}

        <!-- Action Buttons -->
        <div class="dialog-actions">
          {#if !isCreateMode}
            <button
              type="button"
              class="btn btn-danger-ghost"
              onclick={handleDeleteClick}
              disabled={isSaving}
            >
              Delete
            </button>
          {/if}
          <div class="dialog-actions-right">
            <button
              type="button"
              class="btn btn-ghost"
              onclick={closeDialog}
              disabled={isSaving}
            >
              Cancel
            </button>
            <button type="submit" class="btn btn-primary" disabled={isSaving}>
              {isSaving ? "Saving…" : "OK"}
            </button>
          </div>
        </div>
      </form>
    {/if}
  </div>
{/if}

<!-- =========================================================================
     Styles
     ========================================================================= -->
<style>
  /* ── Grid ───────────────────────────────────────────────────────────────── */
  :global(.ag-theme-alpine) {
    height: 100% !important;
    width: 100% !important;
  }

  .toolbar {
    display: flex;
    justify-content: flex-end;
    margin-top: 12px;
    padding: 0 12px 12px;
  }

  .drop-hint {
    padding: 0 12px 12px;
    color: #64748b;
    font-size: 0.78rem;
    text-align: right;
  }

  /* ── Backdrop ───────────────────────────────────────────────────────────── */
  .dialog-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.55);
    backdrop-filter: blur(4px);
    z-index: 40;
    animation: fadeIn 0.15s ease;
  }

  /* ── Panel ──────────────────────────────────────────────────────────────── */
  .dialog-panel {
    position: fixed;
    top: 50%;
    left: 50%;
    translate: -50% -50%;
    z-index: 50;
    width: min(760px, calc(100vw - 2rem));
    max-height: calc(100vh - 4rem);
    overflow-y: auto;
    border-radius: 16px;
    background: rgba(22, 28, 45, 0.9);
    border: 1px solid rgba(255, 255, 255, 0.12);
    box-shadow:
      0 32px 64px rgba(0, 0, 0, 0.5),
      0 0 0 1px rgba(255, 255, 255, 0.05) inset;
    backdrop-filter: blur(20px) saturate(180%);
    animation: slideUp 0.2s cubic-bezier(0.34, 1.56, 0.64, 1);
    color: #e2e8f0;
    font-family: "Inter", "Segoe UI", sans-serif;
  }

  @keyframes fadeIn {
    from {
      opacity: 0;
    }
    to {
      opacity: 1;
    }
  }
  @keyframes slideUp {
    from {
      opacity: 0;
      transform: translate(-50%, calc(-50% + 24px));
    }
    to {
      opacity: 1;
      transform: translate(-50%, -50%);
    }
  }

  /* ── Header ─────────────────────────────────────────────────────────────── */
  .dialog-header {
    padding: 24px 28px 0;
  }
  .dialog-title {
    font-size: 1.2rem;
    font-weight: 700;
    color: #f1f5f9;
    margin: 0 0 4px;
    letter-spacing: -0.02em;
  }
  /* ── Form ───────────────────────────────────────────────────────────────── */
  .dialog-form {
    padding: 20px 28px 24px;
    display: flex;
    flex-direction: column;
    gap: 14px;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 5px;
    flex: 1;
  }
  .field-full {
    width: 100%;
  }
  .field-row {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 12px;
  }

  @media (max-width: 960px) {
    .field-row {
      grid-template-columns: repeat(2, 1fr);
    }
  }

  @media (max-width: 640px) {
    .field-row {
      grid-template-columns: 1fr;
    }
  }

  .field-label {
    font-size: 0.78rem;
    font-weight: 600;
    color: #94a3b8;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    display: flex;
    align-items: baseline;
    gap: 6px;
  }
  .required {
    color: #f87171;
  }
  .field-hint {
    font-size: 0.7rem;
    font-weight: 400;
    text-transform: none;
    letter-spacing: 0;
    color: #4b5563;
  }
  .field-hint code {
    font-family: "JetBrains Mono", monospace;
    background: rgba(255, 255, 255, 0.06);
    padding: 1px 4px;
    border-radius: 3px;
  }

  .field-input {
    background: rgba(255, 255, 255, 0.05);
    border: 1px solid rgba(255, 255, 255, 0.1);
    border-radius: 8px;
    padding: 8px 12px;
    color: #f1f5f9;
    font-size: 0.875rem;
    outline: none;
    transition:
      border-color 0.15s,
      box-shadow 0.15s;
    width: 100%;
    box-sizing: border-box;
  }
  .field-input:focus {
    border-color: #6366f1;
    box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.25);
  }
  .field-input::placeholder {
    color: #475569;
  }
  .field-mono {
    font-family: "JetBrains Mono", "Cascadia Code", "Consolas", monospace;
    font-size: 0.82rem;
  }

  .input-with-button {
    display: flex;
    gap: 8px;
    align-items: center;
  }
  .input-with-button .field-input {
    flex: 1;
  }

  /* number input */
  .number-input-wrap {
    position: relative;
  }
  .field-number {
    -moz-appearance: textfield;
    appearance: textfield;
    text-align: center;
    padding-right: 8px;
  }
  .field-number::-webkit-inner-spin-button,
  .field-number::-webkit-outer-spin-button {
    opacity: 1;
    cursor: pointer;
  }

  /* ── Checkboxes ─────────────────────────────────────────────────────────── */
  .checkbox-row {
    display: flex;
    gap: 24px;
    padding: 4px 0;
  }
  .checkbox-label {
    display: flex;
    align-items: center;
    gap: 8px;
    cursor: pointer;
    color: #cbd5e1;
    font-size: 0.88rem;
    font-weight: 500;
    user-select: none;
  }
  .checkbox {
    width: 16px;
    height: 16px;
    cursor: pointer;
    accent-color: #6366f1;
  }

  /* ── Error banner ───────────────────────────────────────────────────────── */
  .error-banner {
    background: rgba(239, 68, 68, 0.15);
    border: 1px solid rgba(239, 68, 68, 0.4);
    border-radius: 8px;
    color: #fca5a5;
    font-size: 0.82rem;
    padding: 8px 12px;
  }

  /* ── Delete confirm ─────────────────────────────────────────────────────── */
  .confirm-delete-area {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 16px;
    padding: 32px 28px;
    text-align: center;
  }
  .confirm-icon {
    font-size: 2.5rem;
  }
  .confirm-text {
    color: #cbd5e1;
    font-size: 0.95rem;
    line-height: 1.6;
    margin: 0;
  }
  .confirm-text strong {
    color: #f87171;
  }
  .confirm-buttons {
    display: flex;
    gap: 12px;
    margin-top: 8px;
  }

  /* ── Buttons ────────────────────────────────────────────────────────────── */
  .dialog-actions {
    display: flex;
    justify-content: flex-end;
    align-items: center;
    padding-top: 8px;
    gap: 8px;
  }
  .dialog-actions > .btn-danger-ghost {
    margin-right: auto;
  }
  .dialog-actions-right {
    display: flex;
    gap: 8px;
  }

  .btn {
    padding: 8px 18px;
    border-radius: 8px;
    font-size: 0.875rem;
    font-weight: 600;
    cursor: pointer;
    border: 1px solid transparent;
    transition:
      background 0.15s,
      border-color 0.15s,
      transform 0.1s,
      opacity 0.15s;
    outline: none;
    white-space: nowrap;
  }
  .btn-compact {
    padding: 8px 12px;
  }
  .btn:active {
    transform: scale(0.97);
  }
  .btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .btn-primary {
    background: #6366f1;
    color: #fff;
    border-color: #6366f1;
  }
  .btn-primary:hover:not(:disabled) {
    background: #4f46e5;
    border-color: #4f46e5;
  }

  .btn-ghost {
    background: transparent;
    color: #94a3b8;
    border-color: rgba(255, 255, 255, 0.1);
  }
  .btn-ghost:hover:not(:disabled) {
    background: rgba(255, 255, 255, 0.06);
    color: #e2e8f0;
  }

  .btn-danger {
    background: #ef4444;
    color: #fff;
    border-color: #ef4444;
  }
  .btn-danger:hover:not(:disabled) {
    background: #dc2626;
    border-color: #dc2626;
  }

  .btn-danger-ghost {
    background: transparent;
    color: #f87171;
    border-color: rgba(248, 113, 113, 0.3);
    padding: 8px 14px;
  }
  .btn-danger-ghost:hover:not(:disabled) {
    background: rgba(239, 68, 68, 0.12);
  }
</style>
