package main

import (
	"fmt"
)

func main() {

	go func() {
		fmt.Println("naked goroutine - 进程会在 panic 时崩溃")
	}()

}
