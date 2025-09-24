# Patch My PC Network Tester

A network connectivity testing tool for Patch My PC services with both GUI and CLI interfaces.

## Features

- **Graphical User Interface (GUI)**: Default mode with an intuitive interface
- **Dark Mode Support**: Toggle between light and dark themes
- **Command Line Interface (CLI)**: Available via `/nogui` flag
- **Real-time Testing**: Live progress updates and results display
- **Network Connectivity Testing**: Tests connections to Patch My PC services and domains

## Usage

### GUI Mode (Default)
Run the application without any arguments to launch the graphical interface:
```
PMPC-NetworkTester
```

The GUI includes:
- Dark/Light mode toggle
- Start Network Test button
- Progress bar with status updates
- Real-time results list
- All testing runs within the application window

### CLI Mode
Run with the `/nogui` flag for command-line operation:
```
PMPC-NetworkTester /nogui
```

## DISCLAIMER

The information contained in this online site is presented for general educational and information purposes only. The information contained in this site should not be considered exhaustive and the user should seek the advice of appropriate professionals.

In no event shall PMP or its employees be liable for any liability, loss, injury or risk (including, without limitation, incidental and consequential damages, personal injury/wrongful death, lost profits or damages) which is incurred or suffered as a direct or indirect result of the use of any of the material, advice, guidance or services on this site, whether based on warranty, contract, tort, or any other legal theory and whether or not it is is advised of the possibility of such damages.

Patch My PC provides scripts, macro, and other code examples for illustration only, without warranty either expressed or implied, including but not limited to the implied warranties of merchantability and/or fitness for a particular purpose. This script is provided 'AS IS' and Patch My PC does not guarantee that the following script, macro, or code can or should be used in any situation or that operation of the code will be error-free.

## Compile Instructions Windows

For GUI version:
```go
go build -o PMPC-NetworkTester.exe
```

For Windows GUI without console window:
```go
go build -o PMPC-NetworkTester.exe -ldflags -H=windowsgui
```

## Dependencies

- Go 1.19+
- Fyne v2 (for GUI functionality)
- Platform-specific GUI libraries (automatically handled by Fyne)
