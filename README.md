# systats

Go module to get linux system stats. 

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