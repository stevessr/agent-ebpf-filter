package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// ---- moved from app/sessionhandlersshell.go ----

// ServeShellSessionsWS broadcasts shell session list updates over WebSocket.
func ServeShellSessionsWS(c *gin.Context) {
	conn, err := Deps.Upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	notifyCh := Deps.ShellSessions.Subscribe()
	defer Deps.ShellSessions.Unsubscribe(notifyCh)

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
		list := Deps.ShellSessions.List()
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

// HandleCreateShellSession creates a new shell session.
func HandleCreateShellSession(c *gin.Context) {
	var req struct {
		Shell   string            `json:"shell"`
		Command string            `json:"command,omitempty"`
		Args    []string          `json:"args,omitempty"`
		Env     map[string]string `json:"env,omitempty"`
		Label   string            `json:"label,omitempty"`
		WorkDir string            `json:"workDir,omitempty"`
		Cols    int               `json:"cols,omitempty"`
		Rows    int               `json:"rows,omitempty"`
		Kind    string            `json:"kind,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	deps := Deps.MakeShellDeps()
	session, err := Deps.ShellSessions.NewSession(req, deps)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, session)
}

// HandleListShellSessions lists all shell sessions.
func HandleListShellSessions(c *gin.Context) {
	c.JSON(http.StatusOK, Deps.ShellSessions.List())
}

// HandleDeleteShellSession deletes a shell session by ID.
func HandleDeleteShellSession(c *gin.Context) {
	if err := Deps.ShellSessions.Delete(c.Param("id")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// HandleSendShellSessionInput sends input data to a shell session.
func HandleSendShellSessionInput(c *gin.Context) {
	sessionID := strings.TrimSpace(c.Param("id"))
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session id is required"})
		return
	}

	var req struct {
		Data string `json:"data"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	data := []byte(req.Data)
	if len(data) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "data is required"})
		return
	}

	if err := Deps.ShellSessions.SendInput(sessionID, data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// HandleShellSessionsCleanup removes all closed sessions.
func HandleShellSessionsCleanup(c *gin.Context) {
	Deps.ShellSessions.ClearClosed()
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
