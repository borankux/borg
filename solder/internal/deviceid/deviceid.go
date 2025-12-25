package deviceid

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const deviceIDFileName = ".device_id"

// GetOrGenerateDeviceID gets the device ID from cache file or generates a new one based on hardware
func GetOrGenerateDeviceID(cacheDir string) (string, error) {
	cachePath := filepath.Join(cacheDir, deviceIDFileName)
	
	// Try to read cached device ID
	if data, err := os.ReadFile(cachePath); err == nil {
		deviceID := strings.TrimSpace(string(data))
		if deviceID != "" && len(deviceID) == 64 { // SHA256 hex is 64 chars
			return deviceID, nil
		}
	}
	
	// Generate new device ID based on hardware
	deviceID, err := generateDeviceID()
	if err != nil {
		return "", fmt.Errorf("failed to generate device ID: %w", err)
	}
	
	// Cache the device ID
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}
	
	if err := os.WriteFile(cachePath, []byte(deviceID), 0600); err != nil {
		return "", fmt.Errorf("failed to cache device ID: %w", err)
	}
	
	return deviceID, nil
}

// generateDeviceID generates a unique device ID based on hardware information
func generateDeviceID() (string, error) {
	var hardwareInfo strings.Builder
	
	// Collect hardware identifiers
	identifiers := []string{}
	
	// CPU Model (most stable identifier)
	if cpuModel, err := getCPUModel(); err == nil && cpuModel != "" {
		identifiers = append(identifiers, "cpu:"+cpuModel)
	}
	
	// Motherboard/System UUID (very stable)
	if systemUUID, err := getSystemUUID(); err == nil && systemUUID != "" {
		identifiers = append(identifiers, "uuid:"+systemUUID)
	}
	
	// MAC Address (first non-virtual network interface)
	if macAddr, err := getPrimaryMACAddress(); err == nil && macAddr != "" {
		identifiers = append(identifiers, "mac:"+macAddr)
	}
	
	// Machine ID (Linux) or ComputerGUID (Windows)
	if machineID, err := getMachineID(); err == nil && machineID != "" {
		identifiers = append(identifiers, "machine:"+machineID)
	}
	
	// Serial Number (if available)
	if serial, err := getSystemSerial(); err == nil && serial != "" {
		identifiers = append(identifiers, "serial:"+serial)
	}
	
	// Combine all identifiers
	hardwareInfo.WriteString(strings.Join(identifiers, "|"))
	
	if hardwareInfo.Len() == 0 {
		return "", fmt.Errorf("could not collect any hardware identifiers")
	}
	
	// Generate SHA256 hash of hardware info
	hash := sha256.Sum256([]byte(hardwareInfo.String()))
	return hex.EncodeToString(hash[:]), nil
}

// getCPUModel gets CPU model name
func getCPUModel() (string, error) {
	switch runtime.GOOS {
	case "windows":
		cmd := exec.Command("wmic", "cpu", "get", "name", "/format:value")
		output, err := cmd.Output()
		if err != nil {
			return "", err
		}
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "Name=") {
				return strings.TrimSpace(strings.TrimPrefix(line, "Name=")), nil
			}
		}
		return "", fmt.Errorf("could not parse CPU name")
		
	case "linux":
		cmd := exec.Command("grep", "-m", "1", "model name", "/proc/cpuinfo")
		output, err := cmd.Output()
		if err != nil {
			return "", err
		}
		line := strings.TrimSpace(string(output))
		if strings.HasPrefix(line, "model name") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1]), nil
			}
		}
		return "", fmt.Errorf("could not parse CPU model")
		
	case "darwin":
		cmd := exec.Command("sysctl", "-n", "machdep.cpu.brand_string")
		output, err := cmd.Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(output)), nil
		
	default:
		return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// getSystemUUID gets system UUID (motherboard UUID)
func getSystemUUID() (string, error) {
	switch runtime.GOOS {
	case "windows":
		// Get System UUID
		cmd := exec.Command("wmic", "csproduct", "get", "UUID", "/format:value")
		output, err := cmd.Output()
		if err != nil {
			return "", err
		}
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "UUID=") {
				uuid := strings.TrimSpace(strings.TrimPrefix(line, "UUID="))
				// Remove braces if present
				uuid = strings.Trim(uuid, "{}")
				if uuid != "" && uuid != "FFFFFFFF-FFFF-FFFF-FFFF-FFFFFFFFFFFF" {
					return uuid, nil
				}
			}
		}
		return "", fmt.Errorf("could not parse system UUID")
		
	case "linux":
		// Try DMI UUID first
		cmd := exec.Command("cat", "/sys/class/dmi/id/product_uuid")
		output, err := cmd.Output()
		if err == nil {
			uuid := strings.TrimSpace(string(output))
			if uuid != "" && uuid != "FFFFFFFF-FFFF-FFFF-FFFF-FFFFFFFFFFFF" {
				return uuid, nil
			}
		}
		// Fallback to machine-id
		return getMachineID()
		
	case "darwin":
		// macOS: Use system_profiler to get hardware UUID
		cmd := exec.Command("system_profiler", "SPHardwareDataType")
		output, err := cmd.Output()
		if err != nil {
			return "", err
		}
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "Hardware UUID:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					return strings.TrimSpace(parts[1]), nil
				}
			}
		}
		return "", fmt.Errorf("could not find hardware UUID")
		
	default:
		return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// getPrimaryMACAddress gets the MAC address of the first non-virtual network interface
func getPrimaryMACAddress() (string, error) {
	switch runtime.GOOS {
	case "windows":
		// Get MAC address using getmac command
		cmd := exec.Command("getmac", "/fo", "csv", "/nh")
		output, err := cmd.Output()
		if err != nil {
			return "", err
		}
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			// CSV format: "Connection Name,Network Adapter,Physical Address,Transport Name"
			parts := strings.Split(line, ",")
			if len(parts) >= 3 {
				mac := strings.TrimSpace(strings.Trim(parts[2], "\""))
				// Skip virtual adapters (VMware, VirtualBox, etc.)
				if !strings.Contains(strings.ToLower(parts[1]), "virtual") &&
					!strings.Contains(strings.ToLower(parts[1]), "vmware") &&
					!strings.Contains(strings.ToLower(parts[1]), "virtualbox") &&
					mac != "" && mac != "N/A" {
					return mac, nil
				}
			}
		}
		return "", fmt.Errorf("could not find primary MAC address")
		
	case "linux":
		// Get MAC from first non-virtual interface
		cmd := exec.Command("ip", "link", "show")
		output, err := cmd.Output()
		if err != nil {
			return "", err
		}
		lines := strings.Split(string(output), "\n")
		for i, line := range lines {
			if strings.Contains(line, "link/ether") {
				parts := strings.Fields(line)
				for j, part := range parts {
					if part == "link/ether" && j+1 < len(parts) {
						mac := parts[j+1]
						// Check if previous line has interface name (skip lo, docker, veth, etc.)
						if i > 0 {
							prevLine := lines[i-1]
							if !strings.Contains(prevLine, "lo:") &&
								!strings.Contains(prevLine, "docker") &&
								!strings.Contains(prevLine, "veth") &&
								!strings.Contains(prevLine, "br-") {
								return mac, nil
							}
						}
					}
				}
			}
		}
		return "", fmt.Errorf("could not find primary MAC address")
		
	case "darwin":
		// Get MAC from en0 (primary Ethernet/WiFi)
		cmd := exec.Command("ifconfig", "en0")
		output, err := cmd.Output()
		if err != nil {
			// Try en1 if en0 doesn't exist
			cmd = exec.Command("ifconfig", "en1")
			output, err = cmd.Output()
			if err != nil {
				return "", err
			}
		}
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "ether") {
				parts := strings.Fields(line)
				for i, part := range parts {
					if part == "ether" && i+1 < len(parts) {
						return parts[i+1], nil
					}
				}
			}
		}
		return "", fmt.Errorf("could not find MAC address")
		
	default:
		return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// getMachineID gets machine ID (Linux) or ComputerGUID (Windows)
func getMachineID() (string, error) {
	switch runtime.GOOS {
	case "windows":
		// Get ComputerGUID from registry
		cmd := exec.Command("reg", "query", "HKEY_LOCAL_MACHINE\\SOFTWARE\\Microsoft\\Cryptography", "/v", "MachineGuid")
		output, err := cmd.Output()
		if err != nil {
			return "", err
		}
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "MachineGuid") {
				parts := strings.Fields(line)
				for _, part := range parts {
					if len(part) == 36 && strings.Contains(part, "-") {
						return part, nil
					}
				}
			}
		}
		return "", fmt.Errorf("could not find MachineGuid")
		
	case "linux":
		// Read machine-id
		data, err := os.ReadFile("/etc/machine-id")
		if err != nil {
			// Try alternative location
			data, err = os.ReadFile("/var/lib/dbus/machine-id")
			if err != nil {
				return "", err
			}
		}
		return strings.TrimSpace(string(data)), nil
		
	case "darwin":
		// macOS doesn't have a standard machine-id, use hardware UUID instead
		return getSystemUUID()
		
	default:
		return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// getSystemSerial gets system serial number
func getSystemSerial() (string, error) {
	switch runtime.GOOS {
	case "windows":
		cmd := exec.Command("wmic", "bios", "get", "serialnumber", "/format:value")
		output, err := cmd.Output()
		if err != nil {
			return "", err
		}
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "SerialNumber=") {
				serial := strings.TrimSpace(strings.TrimPrefix(line, "SerialNumber="))
				if serial != "" && serial != "To be filled by O.E.M." {
					return serial, nil
				}
			}
		}
		return "", fmt.Errorf("could not find valid serial number")
		
	case "linux":
		// Try DMI serial
		cmd := exec.Command("cat", "/sys/class/dmi/id/product_serial")
		output, err := cmd.Output()
		if err == nil {
			serial := strings.TrimSpace(string(output))
			if serial != "" && serial != "To be filled by O.E.M." {
				return serial, nil
			}
		}
		return "", fmt.Errorf("could not find serial number")
		
	case "darwin":
		// Get serial from system_profiler
		cmd := exec.Command("system_profiler", "SPHardwareDataType")
		output, err := cmd.Output()
		if err != nil {
			return "", err
		}
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "Serial Number (system):") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					return strings.TrimSpace(parts[1]), nil
				}
			}
		}
		return "", fmt.Errorf("could not find serial number")
		
	default:
		return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

