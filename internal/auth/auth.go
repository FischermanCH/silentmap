package auth

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const sessionDuration = 7 * 24 * time.Hour
const cookieName = "sm-session"

// CookieName is the session cookie name used by handlers.
const CookieName = cookieName

type Manager struct {
	mu           sync.RWMutex
	hashFile     string
	passwordHash []byte
	sessions     map[string]time.Time
}

func New(dataDir string) (*Manager, error) {
	m := &Manager{
		hashFile: filepath.Join(dataDir, "auth.hash"),
		sessions: make(map[string]time.Time),
	}
	data, err := os.ReadFile(m.hashFile)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if len(data) > 0 {
		m.passwordHash = bytes.TrimSpace(data)
	}
	return m, nil
}

func (m *Manager) IsSetup() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.passwordHash) > 0
}

func (m *Manager) SetPassword(plain string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := os.WriteFile(m.hashFile, hash, 0600); err != nil {
		return err
	}
	m.mu.Lock()
	m.passwordHash = hash
	m.mu.Unlock()
	return nil
}

func (m *Manager) CheckPassword(plain string) bool {
	m.mu.RLock()
	hash := m.passwordHash
	m.mu.RUnlock()
	if len(hash) == 0 {
		return false
	}
	return bcrypt.CompareHashAndPassword(hash, []byte(plain)) == nil
}

func (m *Manager) NewSession() string {
	b := make([]byte, 32)
	rand.Read(b)
	token := hex.EncodeToString(b)
	m.mu.Lock()
	m.sessions[token] = time.Now().Add(sessionDuration)
	m.mu.Unlock()
	return token
}

func (m *Manager) ValidSession(token string) bool {
	if token == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	exp, ok := m.sessions[token]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(m.sessions, token)
		return false
	}
	return true
}

func (m *Manager) DeleteSession(token string) {
	m.mu.Lock()
	delete(m.sessions, token)
	m.mu.Unlock()
}
