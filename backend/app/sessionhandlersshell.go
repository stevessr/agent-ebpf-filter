package app

import (
	"agent-ebpf-filter/app/shell"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// ---- shell session HTTP handlers (delegate to shell.Manager) ----

func serveShellSessionsWS(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	notifyCh := shellSessions.Subscribe()
	defer shellSessions.Unsubscribe(notifyCh)

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
	var req shell.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	deps := makeShellDeps()
	session, err := shellSessions.NewSession(req, deps)
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

	var req shell.InputRequest
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
