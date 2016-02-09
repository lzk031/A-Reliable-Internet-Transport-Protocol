package main

import (
	"encoding/json"
	"fmt"
	"github.com/cmu440/lsp"
	// "github.com/cmu440/lspnet"
)

func main() {
	mes := lsp.NewConnect()

	temp, _ := json.Marshal(mes)
	fmt.Println(temp)
	fmt.Println(json.Unmarshal(temp, mes))
	fmt.Printf("%+v", mes)
}
