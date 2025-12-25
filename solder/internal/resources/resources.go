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

	// Detect memory
	memoryGB, err := detectMemory()
	if err != nil {
		// Default to 0 if detection fails
		memoryGB = 0
	}
	req.MemoryGB = memoryGB

	// Detect disk space
	diskSpaceGB, err := detectDiskSpace()
	if err != nil {
		// Default to 0 if detection fails
		diskSpaceGB = 0
	}
	req.DiskSpaceGB = diskSpaceGB

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
		// Use wmic to get GPU info on Windows
		cmd := exec.CommandContext(ctx, "wmic", "path", "win32_VideoController", "get", "name,AdapterRAM", "/format:csv")
		output, err := cmd.Output()
		if err != nil {
			// Try alternative method if wmic fails
			cmd = exec.CommandContext(ctx, "powershell", "-Command", "Get-WmiObject Win32_VideoController | Select-Object Name,AdapterRAM | ConvertTo-Json")
			output, err = cmd.Output()
			if err != nil {
				return nil, err
			}
			// Parse JSON output
			var gpuList []struct {
				Name      string `json:"Name"`
				AdapterRAM uint64 `json:"AdapterRAM"`
			}
			if err := json.Unmarshal(output, &gpuList); err == nil {
				for _, gpu := range gpuList {
					memoryGB := float64(gpu.AdapterRAM) / (1024 * 1024 * 1024)
					gpus = append(gpus, client.GPUInfo{
						Name:     gpu.Name,
						MemoryGB: memoryGB,
					})
				}
				return gpus, nil
			}
		} else {
			// Parse CSV output
			lines := strings.Split(string(output), "\n")
			for i, line := range lines {
				if i == 0 || strings.TrimSpace(line) == "" {
					continue
				}
				fields := strings.Split(line, ",")
				if len(fields) < 3 {
					continue
				}
				gpuName := strings.TrimSpace(fields[len(fields)-2])
				if gpuName == "Name" || gpuName == "" {
					continue
				}
				var memoryGB float64
				if memBytes, err := strconv.ParseUint(strings.TrimSpace(fields[len(fields)-1]), 10, 64); err == nil && memBytes > 0 {
					memoryGB = float64(memBytes) / (1024 * 1024 * 1024)
				}
				gpus = append(gpus, client.GPUInfo{
					Name:     gpuName,
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
					currentGPU = client.GPUInfo{
						Name: strings.TrimPrefix(line, "Chipset Model:"),
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

// FillResources fills resource information into a registration request
func FillResources(ctx context.Context, req *client.RegisterRunnerRequest) error {
	resources, err := DetectResources(ctx)
	if err != nil {
		return err
	}

	req.CPUCores = resources.CPUCores
	req.MemoryGB = resources.MemoryGB
	req.DiskSpaceGB = resources.DiskSpaceGB
	req.GPUInfo = resources.GPUInfo
	req.PublicIPs = resources.PublicIPs

	return nil
}

