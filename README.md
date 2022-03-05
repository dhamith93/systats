# systats

Go module to get linux system stats. 

## Usage

Import the module 

```go
import (
	"github.com/dhamith93/systats"
)
```

And use the methods to get the required and supported system stats.

### Memory

```go
func main() {
	memory, err := systats.GetMemory(systats.Megabyte)
	if err != nil {
		panic(err)
	}
	fmt.Println(memory)
}
```