package main

import (
	"github.com/formancehq/stack/components/agent/cmd"
	"github.com/joho/godotenv"
)

//go:generate rm -rf ./dist/operator
//go:generate git clone --depth 1 --branch main https://github.com/formancehq/operator.git ./dist/operator
func main() {
	_ = godotenv.Load()
	cmd.Execute()
}
