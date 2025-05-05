package main

import (
	"github.com/banbox/banbot/entry"
	_ "github.com/banbox/banstrats/adv"
	_ "github.com/banbox/banstrats/grid"
	_ "github.com/banbox/banstrats/ma"
	_ "github.com/banbox/banstrats/rpc_ai"
)

func main() {
	entry.RunCmd()
}
