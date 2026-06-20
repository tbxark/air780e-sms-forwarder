package serialport

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"go.bug.st/serial"
)

type Candidate struct {
	Port   string
	Source string
	Score  int
}

func AutoDetect() (string, error) {
	candidates := Candidates()
	if len(candidates) == 0 {
		return "", fmt.Errorf("no matching ports")
	}
	return candidates[0].Port, nil
}

func Open(portName string, baud int) (serial.Port, error) {
	if baud <= 0 {
		return nil, fmt.Errorf("invalid baud %d", baud)
	}
	return serial.Open(portName, &serial.Mode{
		BaudRate: baud,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	})
}

func PrintCandidates() {
	candidates := Candidates()
	if len(candidates) == 0 {
		slog.Warn("no serial candidates found")
		return
	}

	for _, candidate := range candidates {
		slog.Info("serial candidate", "port", candidate.Port, "score", candidate.Score, "source", candidate.Source)
	}
}

func Candidates() []Candidate {
	var candidates []Candidate
	if runtime.GOOS == "linux" {
		candidates = append(candidates, linuxSerialByIDCandidates()...)
	}
	candidates = append(candidates, serialLibraryCandidates()...)

	return RankCandidates(candidates)
}

func serialLibraryCandidates() []Candidate {
	ports, err := serial.GetPortsList()
	if err != nil {
		slog.Error("list serial ports failed", "err", err)
		return nil
	}
	var candidates []Candidate
	for _, port := range ports {
		candidates = append(candidates, Candidate{
			Port:   port,
			Source: "serial-list",
			Score:  10 + ScorePortName(port) + scoreLinuxTTY(port),
		})
	}
	return candidates
}

func linuxSerialByIDCandidates() []Candidate {
	matches, _ := filepath.Glob("/dev/serial/by-id/*")
	var candidates []Candidate
	for _, link := range matches {
		port, err := filepath.EvalSymlinks(link)
		if err != nil {
			continue
		}
		candidates = append(candidates, Candidate{
			Port:   port,
			Source: "by-id:" + filepath.Base(link),
			Score:  ScorePortName(link) + scoreLinuxTTY(port) + 50,
		})
	}
	return candidates
}

func RankCandidates(candidates []Candidate) []Candidate {
	best := make(map[string]Candidate)
	for _, candidate := range candidates {
		if candidate.Port == "" {
			continue
		}
		if existing, ok := best[candidate.Port]; !ok || candidate.Score > existing.Score {
			best[candidate.Port] = candidate
		}
	}

	ranked := make([]Candidate, 0, len(best))
	for _, candidate := range best {
		ranked = append(ranked, candidate)
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].Score != ranked[j].Score {
			return ranked[i].Score > ranked[j].Score
		}
		return ranked[i].Port < ranked[j].Port
	})
	return ranked
}

func ScorePortName(path string) int {
	name := strings.ToLower(filepath.Base(path))
	score := scoreMarkerEvidence(name)
	if strings.Contains(name, "if03") {
		score += 30
	} else if strings.Contains(name, "if05") {
		score += 10
	}
	if strings.Contains(name, "ttyacm") || strings.Contains(name, "usbmodem") {
		score += 10
	}
	return score
}

func scoreMarkerEvidence(values ...string) int {
	for _, value := range values {
		lower := strings.ToLower(value)
		if strings.Contains(lower, "eigencomm") || strings.Contains(lower, "air780e") || strings.Contains(lower, "air780") || strings.Contains(lower, "luat") {
			return 100
		}
	}
	return 0
}

func scoreLinuxTTY(port string) int {
	if runtime.GOOS != "linux" {
		return 0
	}

	return scoreLinuxTTYInfo(readLinuxTTYUSBInfo(filepath.Base(port)))
}

func scoreLinuxTTYInfo(info linuxTTYUSBInfo) int {
	score := 0
	score += scoreMarkerEvidence(info.Manufacturer, info.Product, info.Interface)
	if strings.EqualFold(info.VendorID, "19d1") && strings.EqualFold(info.ProductID, "0001") {
		score += 150
	}
	switch strings.ToLower(info.Interface) {
	case "at", "usb uart":
		score += 40
	case "log":
		score -= 30
	case "ppp", "rndis":
		score -= 50
	}
	if info.InterfaceNumber == 3 {
		score += 30
	} else if info.InterfaceNumber == 5 {
		score += 10
	} else if info.InterfaceNumber > 5 {
		score -= info.InterfaceNumber
	}
	return score
}

type linuxTTYUSBInfo struct {
	VendorID        string
	ProductID       string
	Manufacturer    string
	Product         string
	Interface       string
	InterfaceNumber int
}

func readLinuxTTYUSBInfo(tty string) linuxTTYUSBInfo {
	var info linuxTTYUSBInfo
	path := filepath.Join("/sys/class/tty", tty, "device")
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return info
	}

	for dir := realPath; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		info.VendorID = firstNonEmpty(info.VendorID, readTrimmed(filepath.Join(dir, "idVendor")))
		info.ProductID = firstNonEmpty(info.ProductID, readTrimmed(filepath.Join(dir, "idProduct")))
		info.Manufacturer = firstNonEmpty(info.Manufacturer, readTrimmed(filepath.Join(dir, "manufacturer")))
		info.Product = firstNonEmpty(info.Product, readTrimmed(filepath.Join(dir, "product")))
		info.Interface = firstNonEmpty(info.Interface, readTrimmed(filepath.Join(dir, "interface")))
		if info.InterfaceNumber == 0 {
			if value := readTrimmed(filepath.Join(dir, "bInterfaceNumber")); value != "" {
				if number, err := strconv.Atoi(value); err == nil {
					info.InterfaceNumber = number
				}
			}
		}
		if info.VendorID != "" && info.ProductID != "" && info.Interface != "" {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return info
}

func readTrimmed(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
