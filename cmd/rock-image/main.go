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
        fmt.Printf("rock-image version %s (built %s, commit %s)\n", Version, BuildTime, GitCommit)
        return
    }
    fmt.Println("rock-image - Placeholder implementation")
    fmt.Println("TODO: Implement rock-image functionality")
}
