package cmd

import "fmt"

// RunSearch implements `sitatame search <pattern>`. Real implementation lands
// in T19; this stub exists so the dispatcher contract is stable.
func RunSearch(env Env, args []string) int {
	fmt.Fprintln(env.Stderr, "sitatame search: unimplemented")
	return 2
}
