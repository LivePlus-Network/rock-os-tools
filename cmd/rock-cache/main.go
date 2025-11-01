package main

import (
    "fmt"
    "os"
)

var (
    Version   = "dev"
    BuildTime = "unknown"
    GitCommit = "unknown"
)

func main() {
    if len(os.Args) > 1 && os.Args[1] == "version" {
        fmt.Printf("rock-cache version %s (built %s, commit %s)\n", Version, BuildTime, GitCommit)
        return
    }
    fmt.Println("rock-cache - Placeholder implementation")
    fmt.Println("TODO: Implement rock-cache functionality")
}
