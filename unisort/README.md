# unisort

Sort integer slices and remove duplicates.

## Install

```sh
go get github.com/rin2yh/gouse/unisort
```

## Usage

```go
import "github.com/rin2yh/gouse/unisort"

unisort.UniqueSortNaturalInts([]int{3, 1, 2, 1, 3}) // [1, 2, 3]
```

## Functions

| Function | Description |
|----------|-------------|
| `UniqueSortNaturalInts(arr []int) []int` | Sorts an integer slice and removes duplicates |
