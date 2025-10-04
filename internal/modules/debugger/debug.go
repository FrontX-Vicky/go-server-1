package debug

import (
	"fmt"
)

// DebugPause prints a variable with a label and waits for Enter before continuing.
// Usage: debug.DebugPause("userID", userID)
func DebugPause(label string, value interface{}) {
	fmt.Printf("[DEBUG] %s = %+v\n", label, value)
	fmt.Println("Press Enter to continue...")
	fmt.Scanln()
}
