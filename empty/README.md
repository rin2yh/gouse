# empty

Empty value checks. Inspired by Dart's `isEmpty` / `isNotEmpty`.

## Install

```sh
go get github.com/rin2yh/gouse/empty
```

## Usage

```go
import "github.com/rin2yh/gouse/empty"

empty.Is(0)        // true
empty.Is("")       // true
empty.Is([]int{})  // true
empty.Is(42)       // false

empty.Any(0, "hello", nil) // true  (0 and nil are empty)
empty.All(0, "", false)    // true  (all are empty)
```

## Functions

| Function | Description |
|----------|-------------|
| `Is(value any) bool` | Returns true if the value is empty |
| `IsNot(value any) bool` | Returns true if the value is not empty |
| `Any(values ...any) bool` | Returns true if any value is empty |
| `All(values ...any) bool` | Returns true if all values are empty |

**Values considered empty:**
- `nil`
- Zero values (`0`, `false`, `""`)
- Empty slices, maps, and channels (`len == 0`)
- Nil pointers and interfaces
