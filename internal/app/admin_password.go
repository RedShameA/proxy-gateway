package app

import (
	"golang.org/x/crypto/bcrypt"
	"net/http"
)

func (g *Gateway) handleAdminPassword(w http.ResponseWriter, r *http.Request) {
	if !g.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	if len(req.NewPassword) < 12 {
		writeError(w, http.StatusBadRequest, validationNewAdminPasswordMinLength)
		return
	}

	// Verify current password
	var hash string
	if err := g.db.QueryRow(`SELECT password_hash FROM admin_credentials LIMIT 1`).Scan(&hash); err != nil {
		writeError(w, http.StatusInternalServerError, "load credential")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.CurrentPassword)); err != nil {
		writeError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}

	// Hash new password
	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "hash password")
		return
	}

	tx, err := g.db.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "update password")
		return
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if _, err := tx.Exec(`UPDATE admin_credentials SET password_hash = ?`, string(newHash)); err != nil {
		writeError(w, http.StatusInternalServerError, "update password")
		return
	}
	if _, err := tx.Exec(`DELETE FROM admin_sessions`); err != nil {
		writeError(w, http.StatusInternalServerError, "invalidate sessions")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "update password")
		return
	}
	committed = true

	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}
