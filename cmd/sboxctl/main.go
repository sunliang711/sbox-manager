package main

import "github.com/sunliang711/sbox-manager/internal/cli"

// main 将 sboxctl 入口交给内部 runner 执行。
func main() {
	cli.RunSboxctl()
}
