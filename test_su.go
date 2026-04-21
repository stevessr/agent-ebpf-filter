package main
import (
    "os/user"
    "fmt"
)
func main() {
    u, err := user.Current()
    fmt.Println(u, err)
}
