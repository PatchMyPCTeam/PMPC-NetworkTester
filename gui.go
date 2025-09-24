package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/PatchMyPCTeam/PMPC-NetworkTester/packages/downloadFile"
	"github.com/PatchMyPCTeam/PMPC-NetworkTester/packages/goCMTrace"
)

type WebGUI struct {
	clients    map[*websocket.Conn]bool
	clientsMux sync.RWMutex
	upgrader   websocket.Upgrader
	results    []connectionResult
	resultsMux sync.RWMutex
}

type ProgressUpdate struct {
	Type     string `json:"type"`
	Current  int    `json:"current"`
	Total    int    `json:"total"`
	Status   string `json:"status"`
	Progress float64 `json:"progress"`
}

type TestResult struct {
	Type   string           `json:"type"`
	Result connectionResult `json:"result"`
}

func NewWebGUI() *WebGUI {
	return &WebGUI{
		clients: make(map[*websocket.Conn]bool),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow connections from any origin
			},
		},
		results: make([]connectionResult, 0),
	}
}

func (w *WebGUI) broadcast(message interface{}) {
	w.clientsMux.RLock()
	defer w.clientsMux.RUnlock()

	data, err := json.Marshal(message)
	if err != nil {
		return
	}

	for client := range w.clients {
		err := client.WriteMessage(websocket.TextMessage, data)
		if err != nil {
			delete(w.clients, client)
			client.Close()
		}
	}
}

func (w *WebGUI) handleWebSocket(rw http.ResponseWriter, r *http.Request) {
	conn, err := w.upgrader.Upgrade(rw, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	w.clientsMux.Lock()
	w.clients[conn] = true
	w.clientsMux.Unlock()

	// Send current results to new client
	w.resultsMux.RLock()
	for _, result := range w.results {
		w.broadcast(TestResult{Type: "result", Result: result})
	}
	w.resultsMux.RUnlock()

	// Keep connection alive
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			w.clientsMux.Lock()
			delete(w.clients, conn)
			w.clientsMux.Unlock()
			break
		}
	}
}

func (w *WebGUI) addResult(result connectionResult) {
	w.resultsMux.Lock()
	w.results = append(w.results, result)
	w.resultsMux.Unlock()
	
	w.broadcast(TestResult{Type: "result", Result: result})
}

func (w *WebGUI) updateProgress(current, total int, status string) {
	var progress float64
	if total > 0 {
		progress = float64(current) / float64(total) * 100
	}
	
	update := ProgressUpdate{
		Type:     "progress",
		Current:  current,
		Total:    total,
		Status:   status,
		Progress: progress,
	}
	
	w.broadcast(update)
}

func (w *WebGUI) connectionTestWeb(connection connectionInfo, wg *sync.WaitGroup) connectionResult {
	defer wg.Done()
	
	// Duplicate the connectionTest logic to avoid WaitGroup conflicts
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
		logMessage := "Failed test for Product: " + result.product + " to connect to: " + result.domainName + " " + result.port + " due to " + result.err.Error()
		logObj.Message = logMessage
		logObj.State = 3
		goCMTrace.LogData(*logObj)
	}
	
	// Add result to GUI
	w.addResult(result)
	
	return result
}

func (w *WebGUI) runNetworkTests() {
	var wg sync.WaitGroup
	
	w.updateProgress(0, 1, "Starting network tests...")
	
	// Test initial connection to Patch My PC
	wg.Add(1)
	connectionObj := connectionInfo{
		product:    "Patch My PC Network Tester",
		domainName: "patchmypc.com",
		port:       "443",
		reason:     "Base Functionality",
	}
	
	state := w.connectionTestWeb(connectionObj, &wg)
	wg.Wait()
	
	if state.result == "Success" {
		w.updateProgress(1, 1, "Downloading domain list...")
		
		fileName, err := downloadFile.DownloadFile("https://patchmypc.com/scupcatalog/downloads/PatchMyPC-DomainList.csv")
		if err != nil {
			w.updateProgress(1, 1, fmt.Sprintf("Error downloading domain list: %v", err))
			return
		}
		
		records, err := readData(fileName)
		if err != nil {
			w.updateProgress(1, 1, fmt.Sprintf("Error reading domain list: %v", err))
			return
		}
		
		// Count valid connections
		totalConnections := 0
		for _, record := range records {
			if len(record) > 5 && shouldConnect(record[2]) {
				totalConnections++
			}
		}
		
		w.updateProgress(0, totalConnections, fmt.Sprintf("Testing %d connections...", totalConnections))
		
		current := 0
		for _, record := range records {
			if len(record) > 5 {
				connectionObj := connectionInfo{
					product:    record[1],
					domainName: record[2],
					port:       record[4],
					reason:     record[5],
				}
				if shouldConnect(connectionObj.domainName) {
					wg.Add(1)
					current++
					w.updateProgress(current, totalConnections, fmt.Sprintf("Testing %s (%d/%d)", connectionObj.domainName, current, totalConnections))
					go w.connectionTestWeb(connectionObj, &wg)
				}
			}
		}
		
		go func() {
			wg.Wait()
			w.updateProgress(totalConnections, totalConnections, "Network tests completed!")
		}()
		
	} else {
		w.updateProgress(1, 1, "Failed to connect to Patch My PC - Cannot progress further.")
	}
}

func (w *WebGUI) handleStart(rw http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		go w.runNetworkTests()
		rw.WriteHeader(http.StatusOK)
		rw.Write([]byte("Tests started"))
	}
}

const htmlTemplate = `
<!DOCTYPE html>
<html>
<head>
    <title>Patch My PC Network Tester</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            margin: 20px;
            background-color: #f5f5f5;
        }
        .container {
            max-width: 1200px;
            margin: 0 auto;
            background-color: white;
            padding: 20px;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        .header {
            text-align: center;
            margin-bottom: 30px;
        }
        .logo {
            width: 200px;
            height: 60px;
            background: linear-gradient(45deg, #2196F3, #21CBF3);
            color: white;
            display: flex;
            align-items: center;
            justify-content: center;
            font-weight: bold;
            font-size: 18px;
            margin: 0 auto 20px;
            border-radius: 4px;
        }
        .progress-section {
            margin-bottom: 30px;
        }
        .progress-bar {
            width: 100%;
            height: 30px;
            background-color: #e0e0e0;
            border-radius: 15px;
            overflow: hidden;
            margin-bottom: 10px;
        }
        .progress-fill {
            height: 100%;
            background: linear-gradient(45deg, #4CAF50, #8BC34A);
            width: 0%;
            transition: width 0.3s ease;
        }
        .status {
            font-size: 16px;
            color: #333;
            margin-bottom: 10px;
        }
        .start-button {
            background: linear-gradient(45deg, #2196F3, #21CBF3);
            color: white;
            border: none;
            padding: 12px 24px;
            font-size: 16px;
            border-radius: 4px;
            cursor: pointer;
            margin-bottom: 20px;
        }
        .start-button:hover {
            opacity: 0.9;
        }
        .start-button:disabled {
            opacity: 0.6;
            cursor: not-allowed;
        }
        .results-section {
            margin-top: 30px;
        }
        .results-header {
            font-size: 18px;
            font-weight: bold;
            margin-bottom: 15px;
            color: #333;
        }
        .results-list {
            max-height: 400px;
            overflow-y: auto;
            border: 1px solid #ddd;
            border-radius: 4px;
        }
        .result-item {
            padding: 8px 12px;
            border-bottom: 1px solid #eee;
            font-family: monospace;
            font-size: 14px;
        }
        .result-item:last-child {
            border-bottom: none;
        }
        .result-success {
            background-color: #f8fff8;
            color: #2e7d32;
        }
        .result-failed {
            background-color: #fff8f8;
            color: #c62828;
        }
        .stats {
            display: flex;
            gap: 20px;
            margin-bottom: 15px;
        }
        .stat {
            padding: 10px;
            border-radius: 4px;
            text-align: center;
            min-width: 80px;
        }
        .stat-success {
            background-color: #e8f5e8;
            color: #2e7d32;
        }
        .stat-failed {
            background-color: #ffeaea;
            color: #c62828;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <div class="logo">PATCH MY PC</div>
            <h1>Network Tester</h1>
        </div>
        
        <div class="progress-section">
            <button id="startButton" class="start-button" onclick="startTests()">Start Network Tests</button>
            <div class="progress-bar">
                <div id="progressFill" class="progress-fill"></div>
            </div>
            <div id="status" class="status">Ready to start network tests...</div>
        </div>
        
        <div class="results-section">
            <div class="results-header">Test Results</div>
            <div class="stats">
                <div class="stat stat-success">
                    <div><strong id="successCount">0</strong></div>
                    <div>Successful</div>
                </div>
                <div class="stat stat-failed">
                    <div><strong id="failedCount">0</strong></div>
                    <div>Failed</div>
                </div>
            </div>
            <div id="resultsList" class="results-list">
                <!-- Results will be populated here -->
            </div>
        </div>
    </div>

    <script>
        let ws;
        let successCount = 0;
        let failedCount = 0;

        function connectWebSocket() {
            ws = new WebSocket('ws://localhost:8080/ws');
            
            ws.onopen = function() {
                console.log('WebSocket connected');
            };
            
            ws.onmessage = function(event) {
                const data = JSON.parse(event.data);
                
                if (data.type === 'progress') {
                    updateProgress(data.progress, data.status);
                } else if (data.type === 'result') {
                    addResult(data.result);
                }
            };
            
            ws.onclose = function() {
                console.log('WebSocket disconnected');
                setTimeout(connectWebSocket, 1000); // Reconnect after 1 second
            };
        }

        function updateProgress(progress, status) {
            document.getElementById('progressFill').style.width = progress + '%';
            document.getElementById('status').textContent = status;
        }

        function addResult(result) {
            const resultsList = document.getElementById('resultsList');
            const resultItem = document.createElement('div');
            resultItem.className = 'result-item ' + (result.result === 'Success' ? 'result-success' : 'result-failed');
            
            const status = result.result === 'Success' ? '✓' : '✗';
            const domain = result.domainName || 'unknown';
            const port = result.port || 'unknown';
            const product = result.product || 'unknown';
            const reason = result.reason || 'unknown';
            resultItem.textContent = status + ' ' + domain + ':' + port + ' (' + product + ') - ' + reason;
            
            resultsList.appendChild(resultItem);
            resultsList.scrollTop = resultsList.scrollHeight;
            
            // Update counters
            if (result.result === 'Success') {
                successCount++;
                document.getElementById('successCount').textContent = successCount;
            } else {
                failedCount++;
                document.getElementById('failedCount').textContent = failedCount;
            }
        }

        function startTests() {
            document.getElementById('startButton').disabled = true;
            successCount = 0;
            failedCount = 0;
            document.getElementById('successCount').textContent = '0';
            document.getElementById('failedCount').textContent = '0';
            document.getElementById('resultsList').innerHTML = '';
            
            fetch('/start', {method: 'POST'})
                .then(response => {
                    if (response.ok) {
                        console.log('Tests started');
                        setTimeout(() => {
                            document.getElementById('startButton').disabled = false;
                        }, 5000); // Re-enable after 5 seconds
                    }
                })
                .catch(error => {
                    console.error('Error starting tests:', error);
                    document.getElementById('startButton').disabled = false;
                });
        }

        // Connect WebSocket when page loads
        window.onload = function() {
            connectWebSocket();
        };
    </script>
</body>
</html>
`

func (w *WebGUI) handleHome(rw http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.New("home").Parse(htmlTemplate))
	tmpl.Execute(rw, nil)
}

func runGUI() {
	webGUI := NewWebGUI()
	
	http.HandleFunc("/", webGUI.handleHome)
	http.HandleFunc("/ws", webGUI.handleWebSocket)
	http.HandleFunc("/start", webGUI.handleStart)
	
	fmt.Println("Starting Patch My PC Network Tester GUI...")
	fmt.Println("Open your web browser and go to: http://localhost:8080")
	fmt.Println("Press Ctrl+C to stop the server")
	
	server := &http.Server{
		Addr:         ":8080",
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	
	log.Fatal(server.ListenAndServe())
}