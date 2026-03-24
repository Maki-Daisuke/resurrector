<script lang="ts">
  import { onMount } from "svelte";
  import { EventsOn } from "../wailsjs/runtime/runtime";

  type AppStateInfo = {
    id: number;
    name: string;
    state: string;
    restartCount: number;
  };

  let apps: Record<number, AppStateInfo> = {};

  onMount(() => {
    EventsOn("app_state_update", (dataStr: string) => {
      try {
        const data: AppStateInfo = JSON.parse(dataStr);
        apps[data.id] = data;
        // Trigger Svelte reactivity
        apps = { ...apps };
      } catch (e) {
        console.error("Failed to parse event:", e);
      }
    });
  });

  $: appList = Object.values(apps).sort((a, b) => a.id - b.id);
</script>

<main class="container">
  <h1>Resurrector</h1>
  <p class="subtitle">ストイックなプロセス監視ツール</p>

  <div class="app-list">
    {#each appList as app}
      <div class="app-card">
        <div class="info">
          <h2>{app.name}</h2>
          <div class="badges">
            <span class="badge state-{app.state.toLowerCase()}">{app.state}</span>
            {#if app.restartCount > 0}
              <span class="badge retry">Retries: {app.restartCount}</span>
            {/if}
          </div>
        </div>
      </div>
    {/each}
    {#if appList.length === 0}
      <div class="empty-state">
        <p>連携待機中...</p>
        <div class="pulse"></div>
      </div>
    {/if}
  </div>
</main>

<style>
  :global(body) {
    background-color: #1b2636;
    color: #e2e8f0;
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
    margin: 0;
    padding: 0;
  }
  .container {
    max-width: 800px;
    margin: 0 auto;
    padding: 2rem;
  }
  h1 {
    margin-bottom: 0.2rem;
    font-size: 2.5rem;
    color: #f8fafc;
    text-shadow: 0 0 10px rgba(148, 163, 184, 0.3);
  }
  .subtitle {
    color: #94a3b8;
    margin-bottom: 2rem;
  }
  .app-list {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }
  .app-card {
    background: #1e293b;
    border: 1px solid #334155;
    border-radius: 8px;
    padding: 1.5rem;
    display: flex;
    justify-content: flex-start;
    gap: 1.5rem;
    align-items: center;
    box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1);
    transition: transform 0.2s, box-shadow 0.2s;
  }
  .app-logo {
    width: 48px;
    height: 48px;
    object-fit: contain;
    border-radius: 8px;
    filter: drop-shadow(0 2px 4px rgba(0,0,0,0.2));
  }
  .app-card:hover {
    transform: translateY(-2px);
    box-shadow: 0 10px 15px -3px rgba(0, 0, 0, 0.2);
  }
  .app-card h2 {
    margin: 0 0 0.5rem 0;
    font-size: 1.3rem;
    color: #f1f5f9;
  }
  .badges {
    display: flex;
    gap: 0.5rem;
  }
  .badge {
    padding: 0.25rem 0.6rem;
    border-radius: 9999px;
    font-size: 0.8rem;
    font-weight: 600;
    letter-spacing: 0.05em;
    text-transform: uppercase;
  }
  .state-running { background: #dcfce7; color: #166534; box-shadow: 0 0 8px rgba(22, 101, 52, 0.3); }
  .state-stopped { background: #f1f5f9; color: #475569; }
  .state-retrying { background: #fef08a; color: #854d0e; }
  .state-failed { background: #fee2e2; color: #991b1b; }
  .badge.retry { background: #ffedd5; color: #c2410c; }
  
  .empty-state {
    text-align: center;
    padding: 3rem;
    background: #1e293b;
    border: 1px dashed #475569;
    border-radius: 8px;
    color: #94a3b8;
  }
  .pulse {
    width: 12px;
    height: 12px;
    background-color: #3b82f6;
    border-radius: 50%;
    margin: 1.5rem auto 0;
    animation: ping 1.5s cubic-bezier(0, 0, 0.2, 1) infinite;
  }
  @keyframes ping {
    75%, 100% { transform: scale(2.5); opacity: 0; }
  }
</style>
