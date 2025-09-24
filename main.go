package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/PatchMyPCTeam/PMPC-NetworkTester/packages/downloadFile"
	"github.com/PatchMyPCTeam/PMPC-NetworkTester/packages/goCMTrace"
)

func connectionTest(connection connectionInfo, wg *sync.WaitGroup, gui *guiState) connectionResult {
	result := connectionResult{}
	timeout := time.Second * 5
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(connection.domainName, connection.port), timeout)
	logObj := new(goCMTrace.LogEntry)
	logObj.File = "PMPC-NetworkTester.log"
	if err != nil {
		result = connectionResult{
			product:    connection.product,
			domainName: connection.domainName,
			port:       connection.port,
			reason:     connection.reason,
			result:     "Failed",
			err:        err,
		}
	}
	if conn != nil {
		defer conn.Close()
		result = connectionResult{
			product:    connection.product,
			domainName: connection.domainName,
			port:       connection.port,
			reason:     connection.reason,
			result:     "Success",
		}
		logMessage := "Successfully tested for Product: " + result.product + " for the reason: " + result.reason + " connected to: " + net.JoinHostPort(result.domainName, result.port)
		logObj.Message = logMessage
		logObj.State = 1
		goCMTrace.LogData(*logObj)
	}
	if result.result == "Failed" {
		logMessage := "Failed test for Product: " + result.product + " to connnect to: " + result.domainName + " " + result.port + " due to " + result.err.Error()
		logObj.Message = logMessage
		logObj.State = 3
		goCMTrace.LogData(*logObj)
	}
	
	// Add result to GUI if available
	if gui != nil {
		gui.addResult(result)
	}
	
	wg.Done()
	return result
}

func shouldConnect(host string) bool {
	state := true
	if host == "localhost" || host == "patchmypc.com" || len(host) == 0 {
		state = false
	}
	return state
}

func readData(fileName string) ([][]string, error) {
	f, err := os.Open(fileName)
	if err != nil {
		return [][]string{}, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	if _, err := r.Read(); err != nil {
		return [][]string{}, err
	}
	records, err := r.ReadAll()
	if err != nil {
		return [][]string{}, err
	}
	return records, nil
}

type connectionInfo struct {
	product    string
	domainName string
	port       string
	reason     string
}

type connectionResult struct {
	product    string
	domainName string
	port       string
	result     string
	reason     string
	err        error
}

// GUI state management
type guiState struct {
	results    *widget.List
	statusText *widget.Label
	progress   *widget.ProgressBar
	darkMode   bool
	resultData []connectionResult
	mutex      sync.Mutex
}

func (g *guiState) addResult(result connectionResult) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.resultData = append(g.resultData, result)
	g.results.Refresh()
}

func (g *guiState) updateStatus(status string) {
	g.statusText.SetText(status)
}

func (g *guiState) setProgress(value float64) {
	g.progress.SetValue(value)
}

func createGUI() (*guiState, func()) {
	myApp := app.New()
	myApp.SetIcon(nil) // Use default icon
	myWindow := myApp.NewWindow("PMPC Network Tester")
	myWindow.Resize(fyne.NewSize(800, 600))

	gui := &guiState{
		darkMode:   false,
		resultData: make([]connectionResult, 0),
	}

	// Create widgets
	gui.statusText = widget.NewLabel("Ready to test network connections...")
	gui.progress = widget.NewProgressBar()
	
	// Dark mode toggle
	darkModeCheck := widget.NewCheck("Dark Mode", func(checked bool) {
		gui.darkMode = checked
		if checked {
			myApp.Settings().SetTheme(theme.DarkTheme())
		} else {
			myApp.Settings().SetTheme(theme.LightTheme())
		}
	})

	// Start test button
	var startButton *widget.Button
	startButton = widget.NewButton("Start Network Test", func() {
		startButton.Disable()
		go func() {
			defer startButton.Enable()
			runNetworkTests(gui)
		}()
	})

	// Results list
	gui.results = widget.NewList(
		func() int {
			gui.mutex.Lock()
			defer gui.mutex.Unlock()
			return len(gui.resultData)
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("Template item")
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			gui.mutex.Lock()
			defer gui.mutex.Unlock()
			if id < len(gui.resultData) {
				result := gui.resultData[id]
				label := item.(*widget.Label)
				status := result.result
				if result.result == "Failed" && result.err != nil {
					status = fmt.Sprintf("Failed: %s", result.err.Error())
				}
				label.SetText(fmt.Sprintf("%s | %s:%s | %s | %s", 
					result.product, result.domainName, result.port, status, result.reason))
				
				// Color code the status
				if result.result == "Success" {
					label.Importance = widget.SuccessImportance
				} else {
					label.Importance = widget.WarningImportance
				}
			}
		},
	)

	// Layout
	content := container.NewBorder(
		container.NewVBox(
			container.NewHBox(darkModeCheck, startButton),
			gui.statusText,
			gui.progress,
		),
		nil,
		nil,
		nil,
		gui.results,
	)

	myWindow.SetContent(content)

	return gui, func() {
		myWindow.ShowAndRun()
	}
}

func runNetworkTests(gui *guiState) {
	var wg sync.WaitGroup
	wg.Add(1)
	
	gui.updateStatus("Testing connection to Patch My PC...")
	gui.setProgress(0.1)
	
	connectionObj := connectionInfo{
		product:    "Patch My PC Network Tester",
		domainName: "patchmypc.com",
		port:       "443",
		reason:     "Base Functionality",
	}
	state := connectionTest(connectionObj, &wg, gui)
	
	if state.result == "Success" {
		gui.updateStatus("Downloading domain list...")
		gui.setProgress(0.3)
		
		fileName, err := downloadFile.DownloadFile("https://patchmypc.com/scupcatalog/downloads/PatchMyPC-DomainList.csv")
		if err != nil {
			gui.updateStatus(fmt.Sprintf("Failed to download domain list: %v", err))
			return
		}
		
		gui.updateStatus("Reading domain list...")
		gui.setProgress(0.4)
		
		records, err := readData(fileName)
		if err != nil {
			gui.updateStatus(fmt.Sprintf("Failed to read domain list: %v", err))
			return
		}

		gui.updateStatus("Testing connections...")
		totalTests := 0
		for _, record := range records {
			connectionObj := connectionInfo{
				product:    record[1],
				domainName: record[2],
				port:       record[4],
				reason:     record[5],
			}
			if shouldConnect(connectionObj.domainName) {
				totalTests++
			}
		}
		
		currentTest := 0
		for _, record := range records {
			connectionObj := connectionInfo{
				product:    record[1],
				domainName: record[2],
				port:       record[4],
				reason:     record[5],
			}
			if shouldConnect(connectionObj.domainName) {
				wg.Add(1)
				go connectionTest(connectionObj, &wg, gui)
				currentTest++
				progress := 0.4 + (0.5 * float64(currentTest) / float64(totalTests))
				gui.setProgress(progress)
			}
		}
	} else {
		gui.updateStatus("Failed to connect to Patch My PC - Cannot progress farther.")
		gui.setProgress(1.0)
		return
	}
	
	wg.Wait()
	gui.updateStatus("Network testing completed!")
	gui.setProgress(1.0)
}

func main() {
	// Check command line arguments for /nogui flag
	useGUI := true
	for _, arg := range os.Args[1:] {
		if strings.ToLower(arg) == "/nogui" {
			useGUI = false
			break
		}
	}

	if useGUI {
		// Run GUI mode
		gui, showFunc := createGUI()
		_ = gui // Keep reference to prevent GC
		showFunc()
	} else {
		// Run CLI mode (original behavior)
		runCLIMode()
	}
}

func runCLIMode() {
	var wg sync.WaitGroup
	wg.Add(1)
	connectionObj := connectionInfo{
		product:    "Patch My PC Network Tester",
		domainName: "patchmypc.com",
		port:       "443",
		reason:     "Base Functionality",
	}
	state := connectionTest(connectionObj, &wg, nil)
	if state.result == "Success" {
		fileName, err := downloadFile.DownloadFile("https://patchmypc.com/scupcatalog/downloads/PatchMyPC-DomainList.csv")
		if err != nil {
			log.Fatal(err)
		}
		records, err := readData(fileName)
		if err != nil {
			log.Fatal(err)
		}

		for _, record := range records {
			connectionObj := connectionInfo{
				product:    record[1],
				domainName: record[2],
				port:       record[4],
				reason:     record[5],
			}
			if shouldConnect(connectionObj.domainName) {
				wg.Add(1)
				go connectionTest(connectionObj, &wg, nil)
			}
		}
	} else {
		fmt.Println("Failed to connect to Patch My PC - Cannot progress farther.")
	}
	wg.Wait()
}
