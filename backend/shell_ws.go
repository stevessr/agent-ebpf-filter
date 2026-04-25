package main

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/creack/pty/v2"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func serveShellWS(c *gin.Context) {
	if sessionID := strings.TrimSpace(c.Query("session_id")); sessionID != "" {
		shellSessions.AttachWS(c)
		return
	}

	serveLegacyShellWS(c)
}

func serveLegacyShellWS(c *gin.Context) {
	shellPath := resolveShellPath(c.DefaultQuery("shell", "auto"))
	if shellPath == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "shell not found"})
		return
	}

	cols, rows := 80, 24
	if v, err := strconv.Atoi(c.DefaultQuery("cols", "80")); err == nil && v > 0 {
		cols = v
	}
	if v, err := strconv.Atoi(c.DefaultQuery("rows", "24")); err == nil && v > 0 {
		rows = v
	}

	cmd := exec.Command(shellPath)
	cmd.Dir = resolveShellWorkDir()
	cmd.Env = setEnvValue(os.Environ(), "TERM", "xterm-256color")

	// Disable fish shell's query-terminal feature to prevent 10s wait warnings
	ff := os.Getenv("fish_features")
	if ff == "" {
		ff = "no-query-term"
	} else if !strings.Contains(ff, "no-query-term") {
		ff = ff + ",no-query-term"
	}
	cmd.Env = setEnvValue(cmd.Env, "fish_features", ff)

	dropPrivileges(cmd)

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Cols: uint16(cols),
		Rows: uint16(rows),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		_ = ptmx.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
		return
	}
	defer func() {
		_ = ptmx.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	go func() {
		defer conn.Close()
		buf := make([]byte, 4096)
		for {
			n, readErr := ptmx.Read(buf)
			if n > 0 {
				payload := append([]byte(nil), buf[:n]...)
				if writeErr := conn.WriteMessage(websocket.BinaryMessage, payload); writeErr != nil {
					return
				}
			}
			if readErr != nil {
				return
			}
		}
	}()

	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			return
		}

		switch messageType {
		case websocket.BinaryMessage:
			if len(data) == 0 {
				continue
			}
			if _, err := ptmx.Write(data); err != nil {
				return
			}
		case websocket.TextMessage:
			var ctrl shellControlMessage
			if err := json.Unmarshal(data, &ctrl); err == nil && ctrl.Type == "resize" {
				if ctrl.Cols > 0 && ctrl.Rows > 0 {
					_ = pty.Setsize(ptmx, &pty.Winsize{
						Cols: uint16(ctrl.Cols),
						Rows: uint16(ctrl.Rows),
					})
				}
				continue
			}
			if _, err := ptmx.Write(data); err != nil {
				return
			}
		}
	}
}
