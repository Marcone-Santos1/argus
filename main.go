// Argus: painel de terminal para rodar várias sessões de agentes (Claude Code,
// opencode) em paralelo, sobre um servidor tmux próprio.
package main

import (
	"fmt"
	"os"
	"time"
)

var startTime = time.Now()

const usageText = `argus: painel para várias sessões de Claude Code e opencode

uso:
  argus              abre o painel (cria o servidor tmux se preciso)
  argus install      instala hooks, config do tmux e config padrão
  argus uninstall    remove tudo o que o install colocou
  argus bench [-s N] mede o desempenho (N segundos por medição, padrão 60)
`

func main() {
	var err error
	if len(os.Args) < 2 {
		err = launch()
	} else {
		switch os.Args[1] {
		case "panel":
			err = runPanel()
		case "install":
			err = install()
		case "uninstall":
			err = uninstall()
		case "bench":
			err = bench(os.Args[2:])
		case "-h", "--help", "help":
			fmt.Print(usageText)
		default:
			fmt.Fprint(os.Stderr, usageText)
			os.Exit(2)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "argus:", err)
		os.Exit(1)
	}
}
