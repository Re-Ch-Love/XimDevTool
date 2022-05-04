package main

import (
	"fmt"
	"github.com/Re-Ch-Love/xim"
	"project/components/counter"
	"project/components/hello_world"
)

func main() {
	xim.SetTitle("Hello Xim")
	router := xim.NewRouter()
	router.RegisterDefault("hello_world", hello_world.HelloWorld)
	router.Register("counter", counter.Counter)
	err := router.Route()
	if err != nil {
		fmt.Println(err)
		return
	}
	xim.Finish()
}
