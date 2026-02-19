# gouse

[![test](https://github.com/YuukiHayashi0510/gouse/actions/workflows/test.yml/badge.svg)](https://github.com/YuukiHayashi0510/gouse/actions/workflows/test.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-brightgreen.svg)](https://opensource.org/licenses/MIT)

Collection of Go utility functions

## Install

```sh
go get github.com/YuukiHayashi0510/gouse
```

## Packages

### `empty` — Empty value checks

```sh
go get github.com/YuukiHayashi0510/gouse/empty
```

| Function | Description |
|----------|-------------|
| `Is(value any) bool` | 値が空かどうかを判定する |
| `IsNot(value any) bool` | 値が空でないかどうかを判定する |
| `Any(values ...any) bool` | いずれかの値が空であれば `true` を返す |
| `All(values ...any) bool` | すべての値が空であれば `true` を返す |

**Empty とみなされる値:**
- `nil`
- ゼロ値 (`0`, `false`, `""`)
- 空のスライス・マップ・チャネル (`len == 0`)
- nil ポインタ・インターフェース

```go
import "github.com/YuukiHayashi0510/gouse/empty"

empty.Is(0)        // true
empty.Is("")       // true
empty.Is([]int{})  // true
empty.Is(42)       // false

empty.Any(0, "hello", nil) // true  (0 と nil が空)
empty.All(0, "", false)    // true  (すべて空)
```

---

### `graceful` — HTTP サーバーのグレースフルシャットダウン

```sh
go get github.com/YuukiHayashi0510/gouse/graceful
```

`SIGINT` / `SIGTERM` を受け取る、またはコンテキストがキャンセルされると、サーバーを安全にシャットダウンします。

```go
import "github.com/YuukiHayashi0510/gouse/graceful"

// シンプルな使い方
srv := &http.Server{Addr: ":8080", Handler: mux}
if err := graceful.Run(ctx, srv, nil); err != nil {
    log.Fatal(err)
}

// シャットダウンタイムアウトとクリーンアップ関数を指定する
if err := graceful.Run(ctx, srv, &graceful.Config{
    ShutdownTimeout: 10 * time.Second,
    Cleanups:        []func(){db.Close},
}); err != nil {
    log.Fatal(err)
}
```

**Config フィールド:**

| フィールド | 型 | デフォルト | 説明 |
|-----------|-----|-----------|------|
| `ShutdownTimeout` | `time.Duration` | `5s` | シャットダウン待機の最大時間 |
| `Cleanups` | `[]func()` | なし | シャットダウン後に順番に実行される関数 |

---

### `unisort` — ユニーク整数のソート

```sh
go get github.com/YuukiHayashi0510/gouse/unisort
```

| Function | Description |
|----------|-------------|
| `UniqueSortNaturalInts(arr []int) []int` | 整数スライスをソートして重複を除去する |

```go
import "github.com/YuukiHayashi0510/gouse/unisort"

unisort.UniqueSortNaturalInts([]int{3, 1, 2, 1, 3}) // [1, 2, 3]
```
