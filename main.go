package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/PMPC-NetworkTester/packages/downloadFile"
	"github.com/PMPC-NetworkTester/packages/goCMTrace"
)

func connectionTest(connection connectionInfo, wg *sync.WaitGroup) connectionResult {
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

func main() {
	var wg sync.WaitGroup
	wg.Add(1)
	connectionObj := connectionInfo{
		product:    "Patch My PC Network Tester",
		domainName: "patchmypc.com",
		port:       "443",
		reason:     "Base Functionality",
	}
	state := connectionTest(connectionObj, &wg)
	if state.result == "Success" {
		fileName := downloadFile.DownloadFile("https://patchmypc.com/scupcatalog/downloads/PatchMyPC-DomainList.csv")
		records, err := readData(fileName.)
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
				go connectionTest(connectionObj, &wg)
			}
		}
	} else {
		fmt.Println("Failed to connect to Patch My PC - Cannot progress farther.")
	}
	wg.Wait()
}
