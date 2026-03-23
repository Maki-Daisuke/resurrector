# Resurrector

Resurrector（リザレクター）は、Windows向けのストイックで極めて軽量なプロセス死活監視・自動再起動ツールです。

バックグラウンドで静かに常駐し、登録されたアプリケーション（プロセス）が予期せず終了した際に、設定された条件に従って自動的に蘇生（再起動）させます。

## ✨ Features

- **ゼロ・ポーリング監視**: Windows API (`WaitForSingleObject`) を利用したイベント駆動型の監視。CPUリソースを無駄に消費しません。
- **極小の常駐メモリ**: 常駐するコアプロセスは純粋なGo言語のみで記述されており、数MBのメモリしか消費しません。
- **オンデマンドのモダンUI**: 設定やステータス確認のUIは、タスクトレイから呼び出された時のみ起動します（Wails + Svelte）。不要な時はプロセスごと終了し、メモリを完全に解放します。
- **堅牢なプロセス間通信 (IPC)**: コアプロセスとUIプロセス間の通信は「標準入出力 (Stdio)」を通じて行われ、ネットワークポートを開かないためセキュアです。
- **ヒューマンリーダブルな設定**: 設定ファイルには人間にも機械にも読み書きしやすい `TOML` フォーマットを採用しています。

## Architecture

Resurrectorは、システムリソースの消費を最小限に抑えるため、**「常駐コアプロセス」**と**「使い捨てのUIプロセス」**の2つの独立したバイナリで構成されています。

### 1. Core Process (`resurrector.exe`)

- **役割**: タスクトレイへの常駐、TOMLファイルの読み込み、子プロセスの起動・死活監視 (`WaitForSingleObject`)。
- **技術**: Go (Pure), `energye/systray`, `golang.org/x/sys/windows`
- **特徴**: UIを持たず、極めて軽量に動作し続けます。タスクトレイから「設定」がクリックされると、UIプロセスを子プロセスとして起動します。

### 2. UI Process (`resurrector-ui.exe`)

- **役割**: ユーザー向けの設定画面、監視ステータスのリアルタイム表示。
- **技術**: Go + Wails + Svelte (TypeScript)
- **特徴**: Wailsのカスタムロガーを用いてフレームワークのログを `STDERR` に逃がし、`STDOUT` を純粋なJSONメッセージング（IPC通信）専用のパイプとして利用します。ウィンドウを閉じるとプロセスは終了します。

## Configuration (`config.toml`)

監視対象のアプリケーションは、実行ファイルと同じディレクトリに配置される `config.toml` で管理されます。

```toml
# Resurrector Configuration

[[app]]
name = "PowerToys Awake"
enabled = true
command = "C:\\Program Files\\PowerToys\\modules\\Awake\\PowerToys.Awake.exe"
args = ["--use-pt-config"]
cwd = "C:\\Program Files\\PowerToys\\modules\\Awake"
restart_delay_sec = 3
healthy_timeout_sec = 60
hide_window = true
max_retries = 5

[[app]]
name = "My Svelte Dev Server"
enabled = false
command = "npm.cmd"
args = ["run", "dev"]
cwd = "C:\\Users\\user\\projects\\my-svelte-app"
restart_delay_sec = 5
healthy_timeout_sec = 60
hide_window = false
max_retries = 3
````

### 項目定義

- `name` (String): UI上に表示される識別用の名前。
- `enabled` (Boolean): `true` の場合、起動時およびUIからの要求時に監視を開始します。
- `command` (String): 実行するコマンドまたは実行ファイルのフルパス。
- `args` (Array of Strings): コマンドに渡す引数のリスト。
- `cwd` (String): コマンドを実行する際の作業ディレクトリ（カレントディレクトリ）。
- `restart_delay_sec` (Integer): プロセス終了を検知してから、再起動を試みるまでの待機時間（秒）。
- `healthy_timeout_sec` (Integer): プロセスが再起動後、この秒数以上安定して稼働し続けた場合にリトライ回数を0にリセットします。
- `hide_window` (Boolean): `true` の場合、プロセスをバックグラウンド（ウィンドウ非表示）で起動します。
- `max_retries` (Integer): 短期間に連続してクラッシュした場合に、監視を停止するまでの最大再起動回数（クラッシュループ対策）。

## Development

### 前提条件

- Go 1.26+
- Node.js 18+ (Svelte用)
- Wails CLI (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`)

### ディレクトリ構成案

```text
.
├── core/                   # 常駐コアプロセス (Pure Go)
│   ├── main.go
│   ├── monitor/            # Windows API 監視ロジック
│   └── tray/               # タスクトレイ制御
├── ui/                     # UIプロセス (Wails)
│   ├── main.go             # Wailsのエントリポイント (STDIO通信ロジック)
│   └── frontend/           # Svelteアプリケーション
└── config.toml             # 設定ファイル
```

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Author

Daisuke (yet another) Maki
