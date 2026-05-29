package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func (m *shellSessionManager) AttachWS(c *gin.Context) {
	sessionID := stringsTrimToDefault(c.Query("session_id"), "")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id is required"})
		return
	}

	session, ok := m.Get(sessionID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "shell session not found"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	backlog, err := session.Attach(conn)
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(err.Error()))
		_ = conn.Close()
		return
	}
	defer session.Detach(conn)

	if len(backlog) > 0 {
		session.writeMu.Lock()
		if err := conn.WriteMessage(websocket.BinaryMessage, backlog); err != nil {
			session.writeMu.Unlock()
			return
		}
		session.writeMu.Unlock()
	}

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
			if err := session.WriteInput(data); err != nil {
				return
			}
		case websocket.TextMessage:
			var ctrl shellControlMessage
			if err := json.Unmarshal(data, &ctrl); err == nil && ctrl.Type == "resize" {
				if ctrl.Cols > 0 && ctrl.Rows > 0 {
					_ = session.Resize(ctrl.Cols, ctrl.Rows)
				}
				continue
			}
			if err := session.WriteInput(data); err != nil {
				return
			}
		}
	}
}

func serveShellSessionsWS(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	notifyCh := shellSessions.subscribe()
	defer shellSessions.unsubscribe(notifyCh)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	sendList := func() {
		list := shellSessions.List()
		data, err := json.Marshal(list)
		if err != nil {
			return
		}
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			return
		}
	}

	sendList()

	for {
		select {
		case <-notifyCh:
			sendList()
		case <-done:
			return
		}
	}
}

func handleCreateShellSession(c *gin.Context) {
	var req ShellSessionCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	session, err := shellSessions.Create(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, session)
}

func handleListShellSessions(c *gin.Context) {
	c.JSON(http.StatusOK, shellSessions.List())
}

func handleDeleteShellSession(c *gin.Context) {
	if err := shellSessions.Delete(c.Param("id")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func handleSendShellSessionInput(c *gin.Context) {
	sessionID := strings.TrimSpace(c.Param("id"))
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session id is required"})
		return
	}

	var req ShellSessionInputRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	data := []byte(req.Data)
	if len(data) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "data is required"})
		return
	}

	if err := shellSessions.SendInput(sessionID, data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func handleShellSessionsCleanup(c *gin.Context) {
	shellSessions.ClearClosed()
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func stringsTrimToDefault(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func normalizeShellSessionKind(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case shellSessionKindTmux:
		return shellSessionKindTmux
	case shellSessionKindScript:
		return shellSessionKindScript
	case shellSessionKindWrapper:
		return shellSessionKindWrapper
	case "", shellSessionKindShell:
		return shellSessionKindShell
	default:
		return shellSessionKindShell
	}
}
