// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"regexp"
	"strings"
	"time"
)

// GELFMessage represents a GELF (Graylog Extended Log Format) message
type GELFMessage struct {
	Version      string                 `json:"version"`
	Host         string                 `json:"host"`
	ShortMessage string                 `json:"short_message"`
	FullMessage  string                 `json:"full_message,omitempty"`
	Timestamp    float64                `json:"timestamp"`
	Level        int                    `json:"level"`
	Facility     string                 `json:"facility,omitempty"`
	Extra        map[string]interface{} `json:"-"`
}

// UnmarshalJSON custom unmarshaler to handle extra fields (fields starting with _)
func (g *GELFMessage) UnmarshalJSON(data []byte) error {
	type Alias GELFMessage
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(g),
	}

	// First unmarshal into a map to capture all fields
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// Then unmarshal into the struct
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Extract extra fields (starting with _)
	g.Extra = make(map[string]interface{})
	for k, v := range raw {
		if len(k) > 0 && k[0] == '_' {
			g.Extra[k] = v
		}
	}

	return nil
}

// ProcessGELF processes a GELF message and converts it to a standard log entry
func (li *LogIngestor) ProcessGELF(gelf GELFMessage) error {
	// Try to parse level from the actual log message first (for JSON or structured logs)
	levelStr := parseLevelFromMessage(gelf.ShortMessage)

	// If we couldn't parse from message, fall back to GELF level (syslog 0-7)
	if levelStr == "" {
		switch gelf.Level {
		case 0, 1, 2: // Emergency, Alert, Critical
			levelStr = "error"
		case 3: // Error
			levelStr = "error"
		case 4: // Warning
			levelStr = "warn"
		case 5: // Notice
			levelStr = "info"
		case 6: // Informational
			levelStr = "info"
		case 7: // Debug
			levelStr = "debug"
		default:
			levelStr = "info"
		}
	}

	// Build structured log entry from GELF
	logMap := make(map[string]interface{})
	logMap["message"] = gelf.ShortMessage
	logMap["level"] = levelStr

	if gelf.Timestamp > 0 {
		// GELF timestamp is Unix timestamp with decimal seconds
		t := time.Unix(int64(gelf.Timestamp), int64((gelf.Timestamp-float64(int64(gelf.Timestamp)))*1e9))
		logMap["timestamp"] = t.Format(time.RFC3339Nano)
	} else {
		logMap["timestamp"] = time.Now().Format(time.RFC3339Nano)
	}

	if gelf.Host != "" {
		logMap["host"] = gelf.Host
	}

	if gelf.FullMessage != "" {
		logMap["full_message"] = gelf.FullMessage
	}

	if gelf.Facility != "" {
		logMap["facility"] = gelf.Facility
	}

	// Add all extra fields (without the leading underscore)
	for k, v := range gelf.Extra {
		// Remove leading underscore from GELF extra fields
		if len(k) > 0 && k[0] == '_' {
			logMap[k[1:]] = v
		} else {
			logMap[k] = v
		}
	}

	// Convert to JSON string and process
	jsonBytes, err := json.Marshal(logMap)
	if err != nil {
		return fmt.Errorf("failed to marshal GELF to JSON: %v", err)
	}

	return li.ProcessLine(string(jsonBytes))
}

// parseLevelFromMessage attempts to extract log level from message content
// Handles both JSON logs and structured text (logrus format)
// Returns empty string if no level found
func parseLevelFromMessage(message string) string {
	// Try 1: Check if message is JSON and extract "level" field
	if strings.HasPrefix(message, "{") {
		var logData map[string]interface{}
		if err := json.Unmarshal([]byte(message), &logData); err == nil {
			if level, ok := logData["level"].(string); ok {
				level = strings.ToLower(level)
				// Normalize variations
				switch level {
				case "warning":
					return "warn"
				case "err":
					return "error"
				case "trace":
					return "debug"
				case "fatal", "panic", "critical":
					return "error"
				case "error", "warn", "info", "debug":
					return level
				}
			}
		}
	}

	// Try 2: Check for logrus text format: level=info
	if strings.Contains(message, "level=") {
		re := regexp.MustCompile(`level=(\w+)`)
		matches := re.FindStringSubmatch(message)
		if len(matches) > 1 {
			level := strings.ToLower(matches[1])
			// Normalize variations
			switch level {
			case "warning":
				return "warn"
			case "err":
				return "error"
			case "trace":
				return "debug"
			case "fatal", "panic", "critical":
				return "error"
			case "error", "warn", "info", "debug":
				return level
			}
		}
	}

	return ""
}

// StartGELFTCPServer starts a TCP server to receive GELF messages from Docker logging driver
func StartGELFTCPServer(addr string, ingestor *LogIngestor) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on TCP: %v", err)
	}
	defer listener.Close()

	log.Printf("GELF TCP server listening on %s", addr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Error accepting connection: %v", err)
			continue
		}

		// Handle each connection in a goroutine
		go handleGELFConnection(conn, ingestor)
	}
}

func handleGELFConnection(conn net.Conn, ingestor *LogIngestor) {
	defer conn.Close()

	// GELF over TCP uses null-terminated messages
	buffer := make([]byte, 0, 8192)
	readBuf := make([]byte, 4096)

	for {
		n, err := conn.Read(readBuf)
		if err != nil {
			if err.Error() != "EOF" {
				log.Printf("Error reading from connection: %v", err)
			}
			return
		}

		buffer = append(buffer, readBuf[:n]...)

		// Process all null-terminated messages in buffer
		for {
			nullIdx := -1
			for i, b := range buffer {
				if b == 0 {
					nullIdx = i
					break
				}
			}

			if nullIdx == -1 {
				// No complete message yet
				break
			}

			// Extract message (excluding null terminator)
			messageBytes := buffer[:nullIdx]
			buffer = buffer[nullIdx+1:]

			// Skip empty messages
			if len(messageBytes) == 0 {
				continue
			}

			// Parse GELF message
			var gelfMsg GELFMessage
			if err := json.Unmarshal(messageBytes, &gelfMsg); err != nil {
				log.Printf("Error parsing GELF message: %v", err)
				continue
			}

			// Process the message
			if err := ingestor.ProcessGELF(gelfMsg); err != nil {
				log.Printf("Error processing GELF: %v", err)
			}
		}
	}
}

// StartGELFUDPServer starts a UDP server to receive GELF messages from Docker logging driver
func StartGELFUDPServer(addr string, ingestor *LogIngestor) error {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %v", err)
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on UDP: %v", err)
	}
	defer conn.Close()

	log.Printf("GELF UDP server listening on %s", addr)

	// Buffer for incoming messages (GELF messages are typically under 8KB)
	buffer := make([]byte, 8192)

	for {
		n, remoteAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			log.Printf("Error reading from UDP: %v", err)
			continue
		}

		// Process GELF message in a goroutine to avoid blocking
		go func(data []byte, addr *net.UDPAddr) {
			var gelfMsg GELFMessage
			if err := json.Unmarshal(data, &gelfMsg); err != nil {
				log.Printf("Error parsing GELF message from %s: %v", addr, err)
				return
			}

			if err := ingestor.ProcessGELF(gelfMsg); err != nil {
				log.Printf("Error processing GELF from %s: %v", addr, err)
			}
		}(buffer[:n], remoteAddr)
	}
}
