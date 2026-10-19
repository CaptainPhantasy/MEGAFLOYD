package megafloydauth

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// DeviceIDFile is the device identifier file name under the home directory.
const DeviceIDFile = "device_id"

// DeviceID returns the stable device identifier stored in <home>/device_id,
// minting and persisting one on first use. The donor client writes the file
// with mode 0600 inside a 0700 home; this does the same.
func DeviceID(homeDir string) (string, error) {
	if homeDir == "" {
		return "", errors.New("megafloydauth: home directory is not configured")
	}
	path := filepath.Join(homeDir, DeviceIDFile)
	if data, err := os.ReadFile(path); err == nil {
		if id := strings.TrimSpace(string(data)); id != "" {
			return id, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read device id: %w", err)
	}

	id, err := newDeviceID()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(homeDir, 0o700); err != nil {
		return "", fmt.Errorf("create home directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(id), 0o600); err != nil {
		return "", fmt.Errorf("write device id: %w", err)
	}
	return id, nil
}

// Headers returns the X-Msh-* identity headers the donor sends on every
// auth request. The server's requirement for these headers is unverified.
func (c Config) Headers() (map[string]string, error) {
	id, err := DeviceID(c.HomeDir)
	if err != nil {
		return nil, err
	}
	version, err := requiredHeader(c.Version, "version")
	if err != nil {
		return nil, err
	}
	platform, err := requiredHeader(c.Platform, "platform")
	if err != nil {
		return nil, err
	}
	hostname, _ := os.Hostname()
	return map[string]string{
		"X-Msh-Platform":     platform,
		"X-Msh-Version":      version,
		"X-Msh-Device-Name":  asciiHeader(hostname, "unknown"),
		"X-Msh-Device-Model": asciiHeader(runtime.GOOS+" "+runtime.GOARCH, "unknown"),
		"X-Msh-Os-Version":   asciiHeader(runtime.GOOS, "unknown"),
		"X-Msh-Device-Id":    id,
	}, nil
}

// UserAgent returns the donor-style product/version string.
func (c Config) UserAgent() (string, error) {
	version, err := requiredHeader(c.Version, "version")
	if err != nil {
		return "", err
	}
	product, err := requiredHeader("megafloyd", "product")
	if err != nil {
		return "", err
	}
	return product + "/" + version, nil
}

func newDeviceID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate device id: %w", err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func asciiHeader(value, fallback string) string {
	cleaned := strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e {
			return -1
		}
		return r
	}, value)
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return fallback
	}
	return cleaned
}

func requiredHeader(value, field string) (string, error) {
	cleaned := asciiHeader(value, "")
	if cleaned == "" {
		return "", fmt.Errorf("megafloydauth: %s must be a non-empty ASCII string", field)
	}
	return cleaned, nil
}
