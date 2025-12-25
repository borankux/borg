package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"borg/solder/internal/client"
)

// DetectResources detects system resources
func DetectResources(ctx context.Context) (*client.RegisterRunnerRequest, error) {
	req := &client.RegisterRunnerRequest{}

	// Detect CPU cores
	req.CPUCores = int32(runtime.NumCPU())

	// Detect CPU model and frequency
	cpuModel, cpuFreq, err := detectCPUInfo()
	if err != nil {
		// Continue without CPU info if detection fails
		cpuModel = ""
		cpuFreq = 0
	}
	req.CPUModel = cpuModel
	req.CPUFrequencyMHz = cpuFreq

	// Detect memory
	memoryGB, err := detectMemory()
	if err != nil {
		// Default to 0 if detection fails
		memoryGB = 0
	}
	req.MemoryGB = memoryGB

	// Detect disk space (free/available)
	diskSpaceGB, err := detectDiskSpace()
	if err != nil {
		// Default to 0 if detection fails
		diskSpaceGB = 0
	}
	req.DiskSpaceGB = diskSpaceGB

	// Detect total disk space
	totalDiskSpaceGB, err := detectTotalDiskSpace()
	if err != nil {
		// Default to 0 if detection fails
		totalDiskSpaceGB = 0
	}
	req.TotalDiskSpaceGB = totalDiskSpaceGB

	// Detect OS version
	osVersion, err := detectOSVersion()
	if err != nil {
		// Continue without OS version if detection fails
		osVersion = ""
	}
	req.OSVersion = osVersion

	// Detect GPU
	gpuInfo, err := detectGPU(ctx)
	if err != nil {
		// Continue without GPU info if detection fails
		gpuInfo = []client.GPUInfo{}
	}
	req.GPUInfo = gpuInfo

	// Detect public IP addresses
	publicIPs, err := detectPublicIPs(ctx)
	if err != nil {
		// Continue without public IPs if detection fails
		publicIPs = []string{}
	}
	req.PublicIPs = publicIPs

	return req, nil
}

// detectMemory detects total system memory in GB
func detectMemory() (float64, error) {
	switch runtime.GOOS {
	case "windows":
		// Use wmic on Windows
		cmd := exec.Command("wmic", "computersystem", "get", "TotalPhysicalMemory")
		output, err := cmd.Output()
		if err != nil {
			return 0, err
		}

		// Parse output: "TotalPhysicalMemory\n1234567890\n"
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		if len(lines) < 2 {
			return 0, fmt.Errorf("unexpected wmic output format")
		}

		memoryBytes, err := strconv.ParseUint(strings.TrimSpace(lines[1]), 10, 64)
		if err != nil {
			return 0, err
		}

		// Convert bytes to GB
		return float64(memoryBytes) / (1024 * 1024 * 1024), nil

	case "linux", "darwin":
		// Use sysctl on macOS/Linux
		var cmd *exec.Cmd
		if runtime.GOOS == "darwin" {
			cmd = exec.Command("sysctl", "-n", "hw.memsize")
		} else {
			cmd = exec.Command("grep", "MemTotal", "/proc/meminfo")
		}

		output, err := cmd.Output()
		if err != nil {
			return 0, err
		}

		if runtime.GOOS == "darwin" {
			// macOS: output is bytes
			memoryBytes, err := strconv.ParseUint(strings.TrimSpace(string(output)), 10, 64)
			if err != nil {
				return 0, err
			}
			return float64(memoryBytes) / (1024 * 1024 * 1024), nil
		} else {
			// Linux: output is "MemTotal: 12345678 kB"
			parts := strings.Fields(string(output))
			if len(parts) < 2 {
				return 0, fmt.Errorf("unexpected meminfo format")
			}

			memoryKB, err := strconv.ParseUint(parts[1], 10, 64)
			if err != nil {
				return 0, err
			}

			// Convert KB to GB
			return float64(memoryKB) / (1024 * 1024), nil
		}

	default:
		return 0, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// detectDiskSpace detects available disk space in GB
func detectDiskSpace() (float64, error) {
	switch runtime.GOOS {
	case "windows":
		// Use wmic to get disk space on Windows
		cmd := exec.Command("wmic", "logicaldisk", "where", "DeviceID='C:'", "get", "FreeSpace", "/format:value")
		output, err := cmd.Output()
		if err != nil {
			return 0, err
		}

		// Parse output: "FreeSpace=1234567890\r\n"
		outputStr := strings.TrimSpace(string(output))
		lines := strings.Split(outputStr, "\n")
		
		for _, line := range lines {
			if strings.HasPrefix(line, "FreeSpace=") {
				freeSpaceStr := strings.TrimPrefix(line, "FreeSpace=")
				freeSpaceStr = strings.TrimSpace(freeSpaceStr)
				freeSpaceBytes, err := strconv.ParseUint(freeSpaceStr, 10, 64)
				if err != nil {
					return 0, err
				}
				// Convert bytes to GB
				return float64(freeSpaceBytes) / (1024 * 1024 * 1024), nil
			}
		}
		
		return 0, fmt.Errorf("could not parse FreeSpace from wmic output")

	case "linux", "darwin":
		// Use df command
		cmd := exec.Command("df", "-BG", "/")
		output, err := cmd.Output()
		if err != nil {
			return 0, err
		}

		// Parse output: "Filesystem 1G-blocks Used Available Use% Mounted on\n/dev/disk... 500G 200G 300G 40% /\n"
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		if len(lines) < 2 {
			return 0, fmt.Errorf("unexpected df output format")
		}

		fields := strings.Fields(lines[1])
		if len(fields) < 4 {
			return 0, fmt.Errorf("unexpected df output format")
		}

		// Available is typically the 4th field (index 3)
		availableStr := strings.TrimSuffix(fields[3], "G")
		availableGB, err := strconv.ParseFloat(availableStr, 64)
		if err != nil {
			return 0, err
		}

		return availableGB, nil

	default:
		return 0, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// detectGPU detects GPU information
func detectGPU(ctx context.Context) ([]client.GPUInfo, error) {
	var gpus []client.GPUInfo

	switch runtime.GOOS {
	case "windows":
		// Use wmic to get GPU info on Windows - get more fields
		cmd := exec.CommandContext(ctx, "wmic", "path", "win32_VideoController", "get", "name,AdapterRAM,AdapterCompatibility", "/format:csv")
		output, err := cmd.Output()
		if err != nil {
			// Try alternative method if wmic fails
			cmd = exec.CommandContext(ctx, "powershell", "-Command", "Get-WmiObject Win32_VideoController | Select-Object Name,AdapterRAM,AdapterCompatibility | ConvertTo-Json")
			output, err = cmd.Output()
			if err != nil {
				return nil, err
			}
			// Parse JSON output
			var gpuList []struct {
				Name                string `json:"Name"`
				AdapterRAM          uint64 `json:"AdapterRAM"`
				AdapterCompatibility string `json:"AdapterCompatibility"`
			}
			if err := json.Unmarshal(output, &gpuList); err == nil {
				for _, gpu := range gpuList {
					if gpu.Name == "" {
						continue
					}
					memoryGB := float64(gpu.AdapterRAM) / (1024 * 1024 * 1024)
					gpuName := gpu.Name
					// Extract brand from name if possible
					if gpu.AdapterCompatibility != "" {
						gpuName = fmt.Sprintf("%s %s", gpu.AdapterCompatibility, gpu.Name)
					}
					gpus = append(gpus, client.GPUInfo{
						Name:     gpuName,
						MemoryGB: memoryGB,
					})
				}
				return gpus, nil
			}
		} else {
			// Parse CSV output
			// Format: Node,AdapterRAM,Name
			lines := strings.Split(string(output), "\n")
			var headerIndex = -1
			var nameIndex = -1
			var ramIndex = -1
			
			// Find header row and column indices
			for i, line := range lines {
				if strings.TrimSpace(line) == "" {
					continue
				}
				fields := strings.Split(line, ",")
				for j, field := range fields {
					field = strings.TrimSpace(field)
					if field == "Name" {
						nameIndex = j
						headerIndex = i
					} else if field == "AdapterRAM" {
						ramIndex = j
					}
				}
				if headerIndex >= 0 && nameIndex >= 0 && ramIndex >= 0 {
					break
				}
			}
			
			// Parse data rows
			for i := headerIndex + 1; i < len(lines); i++ {
				line := strings.TrimSpace(lines[i])
				if line == "" {
					continue
				}
				fields := strings.Split(line, ",")
				if len(fields) <= nameIndex || len(fields) <= ramIndex {
					continue
				}
				
				gpuName := strings.TrimSpace(fields[nameIndex])
				if gpuName == "" || gpuName == "Name" {
					continue
				}
				
				var memoryGB float64
				ramStr := strings.TrimSpace(fields[ramIndex])
				if ramStr != "" && ramStr != "AdapterRAM" {
					if memBytes, err := strconv.ParseUint(ramStr, 10, 64); err == nil && memBytes > 0 {
						memoryGB = float64(memBytes) / (1024 * 1024 * 1024)
					}
				}
				
				// Try to extract brand from name (NVIDIA, AMD, Intel, etc.)
				gpuDisplayName := gpuName
				nameLower := strings.ToLower(gpuName)
				if strings.Contains(nameLower, "nvidia") {
					// Already has NVIDIA in name
					gpuDisplayName = gpuName
				} else if strings.Contains(nameLower, "amd") || strings.Contains(nameLower, "radeon") {
					gpuDisplayName = "AMD " + gpuName
				} else if strings.Contains(nameLower, "intel") {
					gpuDisplayName = "Intel " + gpuName
				}
				
				gpus = append(gpus, client.GPUInfo{
					Name:     gpuDisplayName,
					MemoryGB: memoryGB,
				})
			}
		}

	case "linux":
		// Try nvidia-smi first
		cmd := exec.CommandContext(ctx, "nvidia-smi", "--query-gpu=name,memory.total", "--format=csv,noheader,nounits")
		output, err := cmd.Output()
		if err == nil {
			// Parse nvidia-smi output
			lines := strings.Split(strings.TrimSpace(string(output)), "\n")
			for _, line := range lines {
				parts := strings.Split(line, ",")
				if len(parts) >= 2 {
					name := strings.TrimSpace(parts[0])
					memoryMB, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
					if err == nil {
						gpus = append(gpus, client.GPUInfo{
							Name:     name,
							MemoryGB: memoryMB / 1024,
						})
					}
				}
			}
		}

		// Also try lspci for other GPUs
		cmd = exec.CommandContext(ctx, "lspci")
		output, err = cmd.Output()
		if err == nil {
			lines := strings.Split(string(output), "\n")
			for _, line := range lines {
				if strings.Contains(strings.ToLower(line), "vga") || strings.Contains(strings.ToLower(line), "3d") {
					parts := strings.Split(line, ":")
					if len(parts) >= 3 {
						gpuName := strings.TrimSpace(parts[len(parts)-1])
						// Check if we already have this GPU from nvidia-smi
						found := false
						for _, gpu := range gpus {
							if strings.Contains(gpu.Name, gpuName) || strings.Contains(gpuName, gpu.Name) {
								found = true
								break
							}
						}
						if !found {
							gpus = append(gpus, client.GPUInfo{
								Name:     gpuName,
								MemoryGB: 0, // Unknown
							})
						}
					}
				}
			}
		}

	case "darwin":
		// macOS: Use system_profiler
		cmd := exec.CommandContext(ctx, "system_profiler", "SPDisplaysDataType")
		output, err := cmd.Output()
		if err == nil {
			lines := strings.Split(string(output), "\n")
			var currentGPU client.GPUInfo
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "Chipset Model:") {
					if currentGPU.Name != "" {
						gpus = append(gpus, currentGPU)
					}
					gpuName := strings.TrimSpace(strings.TrimPrefix(line, "Chipset Model:"))
					// Add Apple branding for integrated GPUs
					if !strings.Contains(strings.ToLower(gpuName), "apple") {
						gpuName = "Apple " + gpuName
					}
					currentGPU = client.GPUInfo{
						Name: gpuName,
					}
				} else if strings.HasPrefix(line, "VRAM (Total):") {
					// Parse VRAM like "8 GB"
					parts := strings.Fields(strings.TrimPrefix(line, "VRAM (Total):"))
					if len(parts) > 0 {
						if mem, err := strconv.ParseFloat(parts[0], 64); err == nil {
							currentGPU.MemoryGB = mem
						}
					}
				}
			}
			if currentGPU.Name != "" {
				gpus = append(gpus, currentGPU)
			}
		}
	}

	return gpus, nil
}

// detectPublicIPs detects public IP addresses
func detectPublicIPs(ctx context.Context) ([]string, error) {
	var ips []string

	// Try multiple IP detection services
	services := []string{
		"https://api.ipify.org",
		"https://ifconfig.me/ip",
		"https://icanhazip.com",
		"https://api.ip.sb/ip",
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	for _, service := range services {
		req, err := http.NewRequestWithContext(ctx, "GET", service, nil)
		if err != nil {
			continue
		}

		resp, err := client.Do(req)
		if err != nil {
			continue
		}

		if resp.StatusCode == http.StatusOK {
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				continue
			}

			ip := strings.TrimSpace(string(body))
			if ip != "" {
				// Check if we already have this IP
				found := false
				for _, existingIP := range ips {
					if existingIP == ip {
						found = true
						break
					}
				}
				if !found {
					ips = append(ips, ip)
				}
			}
		}
	}

	return ips, nil
}

// detectCPUInfo detects CPU model and frequency
func detectCPUInfo() (model string, frequencyMHz int32, err error) {
	switch runtime.GOOS {
	case "windows":
		// Get CPU name
		cmd := exec.Command("wmic", "cpu", "get", "name", "/format:value")
		output, err := cmd.Output()
		if err == nil {
			lines := strings.Split(string(output), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "Name=") {
					model = strings.TrimSpace(strings.TrimPrefix(line, "Name="))
					break
				}
			}
		}
		
		// Get CPU max clock speed
		cmd = exec.Command("wmic", "cpu", "get", "maxclockspeed", "/format:value")
		output, err = cmd.Output()
		if err == nil {
			lines := strings.Split(string(output), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "MaxClockSpeed=") {
					freqStr := strings.TrimSpace(strings.TrimPrefix(line, "MaxClockSpeed="))
					if freq, parseErr := strconv.ParseInt(freqStr, 10, 32); parseErr == nil {
						frequencyMHz = int32(freq)
					}
					break
				}
			}
		}
		return model, frequencyMHz, nil

	case "linux":
		// Parse /proc/cpuinfo
		cmd := exec.Command("grep", "-m", "1", "model name", "/proc/cpuinfo")
		output, err := cmd.Output()
		if err == nil {
			line := strings.TrimSpace(string(output))
			if strings.HasPrefix(line, "model name") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					model = strings.TrimSpace(parts[1])
				}
			}
		}
		
		// Get CPU frequency
		cmd = exec.Command("grep", "-m", "1", "cpu MHz", "/proc/cpuinfo")
		output, err = cmd.Output()
		if err == nil {
			line := strings.TrimSpace(string(output))
			if strings.HasPrefix(line, "cpu MHz") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					if freq, parseErr := strconv.ParseFloat(strings.TrimSpace(parts[1]), 32); parseErr == nil {
						frequencyMHz = int32(freq)
					}
				}
			}
		}
		return model, frequencyMHz, nil

	case "darwin":
		// Get CPU brand string
		cmd := exec.Command("sysctl", "-n", "machdep.cpu.brand_string")
		output, err := cmd.Output()
		if err == nil {
			model = strings.TrimSpace(string(output))
		}
		
		// Get CPU frequency (may not be available on all Macs)
		cmd = exec.Command("sysctl", "-n", "hw.cpufrequency")
		output, err = cmd.Output()
		if err == nil {
			if freq, parseErr := strconv.ParseInt(strings.TrimSpace(string(output)), 10, 64); parseErr == nil {
				// Convert Hz to MHz
				frequencyMHz = int32(freq / 1000000)
			}
		}
		return model, frequencyMHz, nil

	default:
		return "", 0, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// detectOSVersion detects OS version with architecture
func detectOSVersion() (string, error) {
	arch := runtime.GOARCH
	archDisplay := ""
	
	// Normalize architecture display
	switch arch {
	case "amd64":
		archDisplay = "x64"
	case "386":
		archDisplay = "x86"
	case "arm64":
		archDisplay = "ARM"
	default:
		archDisplay = arch
	}
	
	switch runtime.GOOS {
	case "windows":
		// Try wmic first
		cmd := exec.Command("wmic", "os", "get", "version", "/format:value")
		output, err := cmd.Output()
		osName := "Windows"
		if err == nil {
			lines := strings.Split(string(output), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "Version=") {
					version := strings.TrimSpace(strings.TrimPrefix(line, "Version="))
					// Format as "Windows 10" or "Windows 11" based on version
					if strings.HasPrefix(version, "10.0.22") {
						osName = "Windows 11"
					} else if strings.HasPrefix(version, "10.0") {
						osName = "Windows 10"
					} else {
						osName = "Windows " + version
					}
					break
				}
			}
		}
		return fmt.Sprintf("%s %s", osName, archDisplay), nil

	case "linux":
		// Parse /etc/os-release for distro name
		distroName := "Linux"
		cmd := exec.Command("grep", "^NAME=", "/etc/os-release")
		output, err := cmd.Output()
		if err == nil {
			line := strings.TrimSpace(string(output))
			if strings.HasPrefix(line, "NAME=") {
				name := strings.Trim(strings.TrimPrefix(line, "NAME="), "\"")
				// Extract short name (Ubuntu, Debian, CentOS, etc.)
				if strings.Contains(strings.ToLower(name), "ubuntu") {
					distroName = "Ubuntu"
				} else if strings.Contains(strings.ToLower(name), "debian") {
					distroName = "Debian"
				} else if strings.Contains(strings.ToLower(name), "centos") {
					distroName = "CentOS"
				} else if strings.Contains(strings.ToLower(name), "fedora") {
					distroName = "Fedora"
				} else if strings.Contains(strings.ToLower(name), "red hat") {
					distroName = "Red Hat"
				} else if strings.Contains(strings.ToLower(name), "arch") {
					distroName = "Arch"
				} else {
					// Use first word of name
					parts := strings.Fields(name)
					if len(parts) > 0 {
						distroName = parts[0]
					}
				}
			}
		}
		
		// Get version if available
		cmd = exec.Command("grep", "^VERSION_ID=", "/etc/os-release")
		output, err = cmd.Output()
		version := ""
		if err == nil {
			line := strings.TrimSpace(string(output))
			if strings.HasPrefix(line, "VERSION_ID=") {
				version = strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), "\"")
			}
		}
		
		if version != "" {
			return fmt.Sprintf("%s %s %s", distroName, version, archDisplay), nil
		}
		return fmt.Sprintf("%s %s", distroName, archDisplay), nil

	case "darwin":
		// Detect chip type (M chip vs Intel)
		chipType := ""
		cmd := exec.Command("sysctl", "-n", "machdep.cpu.brand_string")
		output, err := cmd.Output()
		if err == nil {
			brand := strings.ToLower(strings.TrimSpace(string(output)))
			if strings.Contains(brand, "apple") {
				// Try to get specific chip name
				cmd = exec.Command("sysctl", "-n", "machdep.cpu.brand_string")
				output2, err2 := cmd.Output()
				if err2 == nil {
					fullBrand := strings.TrimSpace(string(output2))
					if strings.Contains(fullBrand, "M1") {
						chipType = "M1"
					} else if strings.Contains(fullBrand, "M2") {
						chipType = "M2"
					} else if strings.Contains(fullBrand, "M3") {
						chipType = "M3"
					} else if strings.Contains(fullBrand, "M4") {
						chipType = "M4"
					} else {
						chipType = "M chip"
					}
				} else {
					chipType = "M chip"
				}
			} else {
				chipType = "Intel"
			}
		} else {
			// Fallback: check architecture
			if arch == "arm64" {
				chipType = "M chip"
			} else {
				chipType = "Intel"
			}
		}
		
		// Get macOS version
		cmd = exec.Command("sw_vers", "-productVersion")
		output, err = cmd.Output()
		version := ""
		if err == nil {
			version = strings.TrimSpace(string(output))
		}
		
		if version != "" {
			return fmt.Sprintf("macOS %s (%s %s)", version, chipType, archDisplay), nil
		}
		return fmt.Sprintf("macOS (%s %s)", chipType, archDisplay), nil

	default:
		return fmt.Sprintf("%s %s", runtime.GOOS, archDisplay), nil
	}
}

// detectTotalDiskSpace detects total disk space in GB
func detectTotalDiskSpace() (float64, error) {
	switch runtime.GOOS {
	case "windows":
		// Use wmic to get total disk size
		cmd := exec.Command("wmic", "logicaldisk", "where", "DeviceID='C:'", "get", "Size", "/format:value")
		output, err := cmd.Output()
		if err != nil {
			return 0, err
		}

		// Parse output: "Size=1234567890\r\n"
		outputStr := strings.TrimSpace(string(output))
		lines := strings.Split(outputStr, "\n")
		
		for _, line := range lines {
			if strings.HasPrefix(line, "Size=") {
				sizeStr := strings.TrimPrefix(line, "Size=")
				sizeStr = strings.TrimSpace(sizeStr)
				sizeBytes, err := strconv.ParseUint(sizeStr, 10, 64)
				if err != nil {
					return 0, err
				}
				// Convert bytes to GB
				return float64(sizeBytes) / (1024 * 1024 * 1024), nil
			}
		}
		
		return 0, fmt.Errorf("could not parse Size from wmic output")

	case "linux", "darwin":
		// Use df command to get total size
		cmd := exec.Command("df", "-BG", "/")
		output, err := cmd.Output()
		if err != nil {
			return 0, err
		}

		// Parse output: "Filesystem 1G-blocks Used Available Use% Mounted on\n/dev/disk... 500G 200G 300G 40% /\n"
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		if len(lines) < 2 {
			return 0, fmt.Errorf("unexpected df output format")
		}

		fields := strings.Fields(lines[1])
		if len(fields) < 2 {
			return 0, fmt.Errorf("unexpected df output format")
		}

		// Total is typically the 2nd field (index 1)
		totalStr := strings.TrimSuffix(fields[1], "G")
		totalGB, err := strconv.ParseFloat(totalStr, 64)
		if err != nil {
			return 0, err
		}

		return totalGB, nil

	default:
		return 0, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// FillResources fills resource information into a registration request
func FillResources(ctx context.Context, req *client.RegisterRunnerRequest) error {
	resources, err := DetectResources(ctx)
	if err != nil {
		return err
	}

	req.CPUCores = resources.CPUCores
	req.CPUModel = resources.CPUModel
	req.CPUFrequencyMHz = resources.CPUFrequencyMHz
	req.MemoryGB = resources.MemoryGB
	req.DiskSpaceGB = resources.DiskSpaceGB
	req.TotalDiskSpaceGB = resources.TotalDiskSpaceGB
	req.OSVersion = resources.OSVersion
	req.GPUInfo = resources.GPUInfo
	req.PublicIPs = resources.PublicIPs

	return nil
}

