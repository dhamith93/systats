# systats

Go module to get linux system stats. 

[![Go](https://github.com/dhamith93/systats/actions/workflows/go.yml/badge.svg)](https://github.com/dhamith93/systats/workflows/go.yml) [![Go Report Card](https://goreportcard.com/badge/github.com/dhamith93/systats)](https://goreportcard.com/report/github.com/dhamith93/systats)

## Usage

Import the module 

```go
import (
	"github.com/dhamith93/systats"
)

func main() {
    syStats := systats.New()
}
```

And use the methods to get the required and supported system stats.

### Memory

```go
func main() {
	syStats := systats.New()
	memory, err := systats.GetMemory(syStats, systats.Megabyte)
	if err != nil {
		panic(err)
	}
	fmt.Println(memory)
}
```