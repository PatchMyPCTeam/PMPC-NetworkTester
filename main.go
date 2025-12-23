package main

import (
	"github.com/PatchMyPCTeam/PMPC-NetworkTester/packages/gui"
)

func main() {
	// Create and run the GUI application
	app := gui.NewNetworkTesterGUI()
	app.Run()
}
