package main

import (
	"github.com/banbox/banbot/entry"
	_ "github.com/banbox/banstrats/adv"
	_ "github.com/banbox/banstrats/examples/fundingrate"
	_ "github.com/banbox/banstrats/grid"
	_ "github.com/banbox/banstrats/idea"
	_ "github.com/banbox/banstrats/examples/longshort"
	_ "github.com/banbox/banstrats/ma"
	_ "github.com/banbox/banstrats/rpc_ai"
	_ "github.com/banbox/banstrats/tmp"
)

func main() {
	entry.RunCmd()
}
