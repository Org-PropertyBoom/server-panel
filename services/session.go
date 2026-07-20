package services

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const SessionCookieName = "vps_session"

type Session struct {
	ExpiresAt time.Time `json:"expiresAt"`
	Mode      string    `json:"mode"`
	Token     string    `json:"token"`
	UID       int       `json:"uid"`
	Username  string    `json:"username"`
}

type SessionService struct {
	mu       sync.RWMutex
	path     string
	sessions map[string]Session
	ttl      time.Duration
}

func NewSessionService() *SessionService {
	service := &SessionService{
		path:     sessionStorePath(),
		sessions: make(map[string]Session),
		ttl:      24 * time.Hour,
	}
	service.load()
	return service
}

func (s *SessionService) Create(user AuthenticatedUser, mode string) (Session, error) {
	token, err := sessionToken()
	if err != nil {
		return Session{}, err
	}

	session := Session{
		ExpiresAt: time.Now().Add(s.ttl),
		Mode:      mode,
		Token:     token,
		UID:       user.UID,
		Username:  user.Username,
	}

	s.mu.Lock()
	s.sessions[token] = session
	err = s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		return Session{}, err
	}

	return session, nil
}

func (s *SessionService) Get(token string) (Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, exists := s.sessions[token]
	if !exists {
		return Session{}, false
	}
	if time.Now().After(session.ExpiresAt) {
		delete(s.sessions, token)
		_ = s.saveLocked()
		return Session{}, false
	}
	return session, true
}

func (s *SessionService) MaxAge() int {
	return int(s.ttl.Seconds())
}

func sessionToken() (string, error) {
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(token), nil
}

func sessionStorePath() string {
	if path := os.Getenv("SESSION_PATH"); path != "" {
		return path
	}

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), "ppt-server-panel-session")
	}

	return filepath.Join(home, ".ppt-server-panel", "data", "session")
}

func (s *SessionService) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}

	var sessions map[string]Session
	if err := json.Unmarshal(data, &sessions); err != nil {
		return
	}

	now := time.Now()
	for token, session := range sessions {
		if token == "" || now.After(session.ExpiresAt) {
			continue
		}
		if session.Token == "" {
			session.Token = token
		}
		s.sessions[token] = session
	}
}

func (s *SessionService) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s.sessions, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return err
	}

	return os.Rename(tmpPath, s.path)
}
