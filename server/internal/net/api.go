package net

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/kving/games/elements/server/internal/auth"
	"github.com/kving/games/elements/server/internal/store"
)

// AuthHandler serves the REST auth endpoints:
//
//	POST /api/auth/register
//	POST /api/auth/login
//
// Returns a JWT that clients use as ?token= when opening the WebSocket.
type AuthHandler struct {
	store   store.PlayerStore
	authSvc *auth.Service
}

func NewAuthHandler(st store.PlayerStore, svc *auth.Service) *AuthHandler {
	return &AuthHandler{store: st, authSvc: svc}
}

// Register godoc
//
//	POST /api/auth/register
//	Body: {"name":"...","password":"...","element":"fire|water|earth|air|none"}
//	201 Created: {"token":"...","name":"...","element":1,"level":1,"xp":0}
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Name     string `json:"name"`
		Password string `json:"password"`
		Element  string `json:"element"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid request body"))
		return
	}
	if req.Name == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, errBody(ErrCodeInvalidRequest))
		return
	}
	el := parseElement(req.Element)
	p, err := h.store.Register(r.Context(), req.Name, req.Password, uint8(el))
	if errors.Is(err, store.ErrNameTaken) {
		writeJSON(w, http.StatusConflict, errBody(ErrCodeNameTaken))
		return
	}
	if err != nil {
		slog.Error("register failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, errBody(ErrCodeInternalError))
		return
	}
	token, err := h.authSvc.Sign(int64(p.ID))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(ErrCodeInternalError))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"token":   token,
		"name":    p.Name,
		"element": p.Element,
		"level":   p.Level,
		"xp":      p.XP,
	})
}

// Login godoc
//
//	POST /api/auth/login
//	Body: {"name":"...","password":"..."}
//	200 OK: {"token":"...","name":"...","element":1,"level":5,"xp":230}
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(ErrCodeInvalidRequest))
		return
	}
	p, err := h.store.Authenticate(r.Context(), req.Name, req.Password)
	if errors.Is(err, store.ErrBadCredentials) {
		writeJSON(w, http.StatusUnauthorized, errBody(ErrCodeBadCredentials))
		return
	}
	if err != nil {
		slog.Error("login failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, errBody(ErrCodeInternalError))
		return
	}
	token, err := h.authSvc.Sign(int64(p.ID))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(ErrCodeInternalError))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token":   token,
		"name":    p.Name,
		"element": p.Element,
		"level":   p.Level,
		"xp":      p.XP,
	})
}

// Error codes returned to clients. Clients look these up in their own translation
// table — the server never sends human-readable error strings.
const (
	ErrCodeNameTaken       = "name_taken"
	ErrCodeBadCredentials  = "bad_credentials"
	ErrCodeInvalidRequest  = "invalid_request"
	ErrCodeInternalError   = "internal_error"
)

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func errBody(code string) map[string]string { return map[string]string{"error_code": code} }
