# Patch My PC Network Tester

## DISCLAIMER

The information contained in this online site is presented for general educational and information purposes only. The information contained in this site should not be considered exhaustive and the user should seek the advice of appropriate professionals.

In no event shall PMP or its employees be liable for any liability, loss, injury or risk (including, without limitation, incidental and consequential damages, personal injury/wrongful death, lost profits or damages) which is incurred or suffered as a direct or indirect result of the use of any of the material, advice, guidance or services on this site, whether based on warranty, contract, tort, or any other legal theory and whether or not it is is advised of the possibility of such damages.

Patch My PC provides scripts, macro, and other code examples for illustration only, without warranty either expressed or implied, including but not limited to the implied warranties of merchantability and/or fitness for a particular purpose. This script is provided 'AS IS' and Patch My PC does not guarantee that the following script, macro, or code can or should be used in any situation or that operation of the code will be error-free.

## Usage

The Patch My PC Network Tester can be run in two modes:

### Command Line Interface (CLI) Mode
Run the application without any flags for traditional console output:

```
PMPC-NetworkTester.exe
```

### Graphical User Interface (GUI) Mode
Run the application with the `-gui` flag to launch the web-based GUI:

```
PMPC-NetworkTester.exe -gui
```

This will start a web server on `http://localhost:8080`. Open your web browser and navigate to this address to access the GUI. The GUI features:

- **Patch My PC Logo**: Prominently displayed branding
- **Progress Bar**: Real-time visual indication of test progress
- **Live Status Updates**: Current test status and progress information
- **Test Results**: Live updating list of connection test results with success/failure indicators
- **Statistics**: Summary counters for successful and failed tests

The GUI works on Windows 10, Windows 11, and Windows Server without requiring additional dependencies.

## Compile Instructions Windows

### CLI Version
```go
go build -o PMPC-NetworkTester.exe
```

### GUI Version (same executable, different usage)
```go
go build -o PMPC-NetworkTester.exe -ldflags -H=windowsgui
```

The `-ldflags -H=windowsgui` flag is optional and only affects the console window behavior in GUI mode.
