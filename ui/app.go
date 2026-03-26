package main

import (
	"context"
	"encoding/json"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"resurrector/util"
)

// App struct
type App struct {
	ctx context.Context
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// UI server to receive states via JSON-RPC
type UI struct {
	Ctx context.Context
}

// UpdateState receives the state update from core
func (u *UI) UpdateState(msg map[string]interface{}, reply *bool) error {
	b, _ := json.Marshal(msg)
	runtime.EventsEmit(u.Ctx, "app_state_update", string(b))
	*reply = true
	return nil
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// domReady is called after the frontend DOM has been loaded.
func (a *App) domReady(ctx context.Context) {
	// Start RPC server serving on Stdin/Stdout in the background.
	// We add a tiny delay to give Svelte's onMount enough time to register its EventsOn listener
	// before we consume the buffered initial state from the OS pipe.
	go func() {
		time.Sleep(200 * time.Millisecond)
		
		server := rpc.NewServer()
		server.Register(&UI{Ctx: ctx})

		conn := &util.StdioConn{ReadCloser: os.Stdin, WriteCloser: os.Stdout}
		server.ServeCodec(jsonrpc.NewServerCodec(conn))
	}()
}
