package terminal

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"os/user"

	"github.com/creack/pty"
	"golang.org/x/net/websocket"

	"ppt/server-panel/services"
)

type wsMessage struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
	Data string `json:"data"`
}

func Handler(sessions *services.SessionService) http.Handler {
	return websocket.Handler(func(ws *websocket.Conn) {
		req := ws.Request()
		cookie, err := req.Cookie(services.SessionCookieName)
		if err != nil {
			_ = ws.Close()
			return
		}

		session, exists := sessions.Get(cookie.Value)
		if !exists || session.Mode != "root" {
			_ = ws.Close()
			return
		}

		targetUsername := req.URL.Query().Get("user")
		var cmd *exec.Cmd
		if targetUsername != "" {
			target, err := user.Lookup(targetUsername)
			if err != nil || target.Username != targetUsername {
				_ = ws.Close()
				return
			}
			cmd = exec.Command("su", "-", targetUsername)
		} else {
			shell := "/bin/bash"
			if _, err := os.Stat(shell); os.IsNotExist(err) {
				shell = "/bin/sh"
			}
			cmd = exec.Command(shell)
		}
		cmd.Env = append(os.Environ(), "TERM=xterm-256color", "USER="+session.Username)

		ptmx, err := pty.Start(cmd)
		if err != nil {
			_ = ws.Close()
			return
		}
		defer func() {
			_ = ptmx.Close()
			_ = cmd.Process.Kill()
		}()

		// Output loop: PTY -> WS
		go func() {
			buf := make([]byte, 2048)
			for {
				n, err := ptmx.Read(buf)
				if err != nil {
					break
				}
				err = websocket.Message.Send(ws, string(buf[:n]))
				if err != nil {
					break
				}
			}
		}()

		// Input loop: WS -> PTY
		for {
			var msg string
			err := websocket.Message.Receive(ws, &msg)
			if err != nil {
				break
			}

			var wsMsg wsMessage
			if err := json.Unmarshal([]byte(msg), &wsMsg); err == nil {
				switch wsMsg.Type {
				case "input":
					_, _ = ptmx.Write([]byte(wsMsg.Data))
				case "resize":
					_ = pty.Setsize(ptmx, &pty.Winsize{
						Cols: wsMsg.Cols,
						Rows: wsMsg.Rows,
					})
				}
			} else {
				_, _ = ptmx.Write([]byte(msg))
			}
		}
	})
}
